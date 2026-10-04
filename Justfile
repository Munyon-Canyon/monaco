set dotenv-load := true
set dotenv-filename := ".env"
set shell := ["bash", "-euo", "pipefail", "-c"]

# Re-exec recipe under dotenvx once (decrypts .env.local into the process).
# Usage inside a recipe body: _dotenvx just <recipe> <args...>
_dotenvx := "./scripts/with-dotenv-local.sh"

default:
    @just --list

# Interactive clone setup. Asks before each install. `just install --check` reports only.
install *flags:
    ./scripts/install-dev.sh {{flags}}

# Encrypt or decrypt repo-root .env.local via dotenvx (file ops — not with-dotenv-local re-exec).
encrypt:
    #!/usr/bin/env bash
    set -euo pipefail
    if [[ ! -f .env.local ]]; then
      echo "error: .env.local missing — copy .env.example to .env.local, or place a teammate encrypted .env.local plus .env.keys in the clone root." >&2
      exit 1
    fi
    if ! command -v dotenvx >/dev/null 2>&1; then
      echo "error: dotenvx not on PATH. Install: https://dotenvx.com/docs/install" >&2
      exit 1
    fi
    dotenvx encrypt -f .env.local
    if [[ -f .env.production ]]; then
      dotenvx encrypt -f .env.production
    fi

decrypt:
    #!/usr/bin/env bash
    set -euo pipefail
    if [[ ! -f .env.local ]]; then
      echo "error: .env.local missing — copy .env.example to .env.local, or place a teammate encrypted .env.local plus .env.keys in the clone root." >&2
      exit 1
    fi
    if ! command -v dotenvx >/dev/null 2>&1; then
      echo "error: dotenvx not on PATH. Install: https://dotenvx.com/docs/install" >&2
      exit 1
    fi
    dotenvx decrypt -f .env.local
    if [[ -f .env.production ]]; then
      dotenvx decrypt -f .env.production
    fi

# Print decrypted .env.local keys/values (.env.production omitted).
show-env:
    #!/usr/bin/env bash
    set -euo pipefail
    {{_dotenvx}} dotenvx get -f .env.local --format eval-export
    if [[ -f .env.production ]]; then
      echo "note: .env.production exists but is omitted (dev default)." >&2
    fi

build app:
    #!/usr/bin/env bash
    set -euo pipefail
    case "{{app}}" in
      backend)
        mkdir -p bin
        (cd apps/backend && go build -o ../../bin/ ./cmd/...)
        echo "built bin/api bin/worker bin/monacoctl"
        ;;
      mobile)
        if [[ ! -d apps/mobile ]]; then
          echo "error: apps/mobile is not scaffolded yet (M0-T4)."
          exit 1
        fi
        # ensure-ios-privy-config reads .env.local via dotenvx get → Privy.local.xcconfig
        ./scripts/ensure-ios-privy-config.sh generate
        ./scripts/ios-build
        ;;
      *)
        echo "error: unknown app '{{app}}' (use backend or mobile)"
        exit 1
        ;;
    esac

test app:
    #!/usr/bin/env bash
    set -euo pipefail
    case "{{app}}" in
      backend)
        ./scripts/require-docker.sh
        docker compose --profile test up -d --wait postgres-test
        scripts/test-backend.sh
        if [[ "${SKIP_SCRIPTS_TESTS:-}" != "1" && -f scripts/go.mod ]]; then
          (cd scripts && go test -short ./...)
        fi
        ;;
      mutation)
        (cd apps/backend && go run ./cmd/monacoctl mutation ${MUTATION_ARGS:-})
        ;;
      vuln)
        if [[ "$(cat .bin/govulncheck.version 2>/dev/null)" != "$(sed -n 's/^version=//p' scripts/install-govulncheck.sh)" ]]; then
          scripts/install-govulncheck.sh
        fi
        (cd apps/backend && ../../.bin/govulncheck ./...)
        ;;
      mobile)
        if [[ ! -d apps/mobile ]]; then
          echo "error: apps/mobile is not scaffolded yet (M0-T4)."
          exit 1
        fi
        if [[ ! -f packages/mobile-core/Package.swift ]]; then
          echo "error: packages/mobile-core is not scaffolded yet."
          exit 1
        fi
        swift format lint --strict --recursive --parallel apps/mobile packages/mobile-core
        scripts/swiftlint-ratchet.sh
        # Host unit tests only (swift test on macOS). iOS sim UI tests stay on just build/run mobile.
        scripts/mobile-core-test.sh
        ;;
      *)
        echo "error: unknown app '{{app}}' (use backend, mutation, vuln or mobile)"
        exit 1
        ;;
    esac

run *app:
    #!/usr/bin/env bash
    set -euo pipefail
    if [[ -z "{{app}}" ]]; then
      just run backend &
      backend_pid=$!
      just run mobile
      echo "Simulator launched. Backend still running; Ctrl+C or just stop backend to stop it."
      wait "$backend_pid"
      exit 0
    fi
    case "{{app}}" in
      backend)
        just build backend
        ./scripts/require-docker.sh
        docker compose up -d --wait postgres nats
        source ./scripts/run-with-logs.sh
        monaco_init_logs
        env -u MONACO_LOG_DIR -u MONACO_DOTENVX {{_dotenvx}} "$PWD/bin/api" > >(tee -a "${MONACO_LOG_DIR}/api.log") 2>&1 &
        api_pid=$!
        env -u MONACO_LOG_DIR -u MONACO_DOTENVX {{_dotenvx}} "$PWD/bin/worker" > >(tee -a "${MONACO_LOG_DIR}/worker.log") 2>&1 &
        worker_pid=$!
        trap 'kill -TERM "$api_pid" "$worker_pid" 2>/dev/null || true' INT TERM
        wait "$api_pid" "$worker_pid"
        ;;
      mobile)
        if [[ "${MONACO_DOTENVX:-}" != "1" ]]; then
          exec {{_dotenvx}} env MONACO_DOTENVX=1 just run mobile
        fi
        if [[ ! -d apps/mobile ]]; then
          echo "error: apps/mobile is not scaffolded yet (M0-T4)."
          exit 1
        fi
        # Privy: xcconfig + SIMCTL_CHILD_* via with-ios-privy-env, then scripts/ios-sim
        source ./scripts/run-with-logs.sh
        monaco_init_logs
        ./scripts/ios-sim 2>&1 | tee -a "${MONACO_LOG_DIR}/mobile.log"
        ;;
      *)
        echo "error: unknown app '{{app}}' (use backend or mobile)"
        exit 1
        ;;
    esac

stop *app:
    #!/usr/bin/env bash
    set -euo pipefail
    if [[ -z "{{app}}" ]]; then
      just stop backend
      just stop mobile
      exit 0
    fi
    case "{{app}}" in
      backend)
        pattern="^${PWD}/bin/(api|worker)$"
        pkill -TERM -f "$pattern" || true
        for _ in $(seq 1 50); do
          pgrep -f "$pattern" >/dev/null || exit 0
          sleep 0.2
        done
        echo "error: api or worker still running 10s after SIGTERM" >&2
        exit 1
        ;;
      mobile)
        ./scripts/stop-mobile.sh
        ;;
      *)
        echo "error: unknown app '{{app}}' (use backend or mobile)"
        exit 1
        ;;
    esac

reset *target:
    #!/usr/bin/env bash
    set -euo pipefail
    if [[ -z "{{target}}" ]]; then
      just stop
      if [[ "${MONACO_DOTENVX:-}" != "1" ]]; then
        exec {{_dotenvx}} env MONACO_DOTENVX=1 just reset
      fi
      ./scripts/reset-db.sh --all
      exit 0
    fi
    case "{{target}}" in
      backend)
        just stop backend
        rm -f bin/api bin/worker bin/monacoctl
        echo "removed bin/api bin/worker bin/monacoctl"
        ;;
      mobile)
        ./scripts/stop-mobile.sh
        if [[ ! -d apps/mobile ]]; then
          echo "error: apps/mobile is not scaffolded yet (M0-T4)."
          exit 1
        fi
        gold_udid="$(./scripts/resolve-ios-sim.sh)"
        xcodebuild -project apps/mobile/Monaco.xcodeproj -scheme Monaco \
          -destination "platform=iOS Simulator,id=${gold_udid}" \
          -derivedDataPath "$(git rev-parse --show-toplevel)/.build/DerivedData" \
          clean
        echo "xcodebuild clean complete"
        ;;
      db)
        if [[ "${MONACO_DOTENVX:-}" != "1" ]]; then
          exec {{_dotenvx}} env MONACO_DOTENVX=1 just reset db
        fi
        ./scripts/reset-db.sh
        ;;
      *)
        echo "error: unknown target '{{target}}' (use backend, mobile, or db)"
        exit 1
        ;;
    esac

# Apply pending migrations to the .env.local database and print its revision. `just run backend` never migrates.
migrate target:
    #!/usr/bin/env bash
    set -euo pipefail
    case "{{target}}" in
      db)
        mkdir -p bin
        (cd apps/backend && go build -o ../../bin/ ./cmd/monacoctl)
        {{_dotenvx}} "$PWD/bin/monacoctl" migrate apply
        {{_dotenvx}} "$PWD/bin/monacoctl" migrate status
        ;;
      *)
        echo "error: unknown target '{{target}}' (use db)"
        exit 1
        ;;
    esac

# Regenerate checked-in files (`just gen docs`) or scaffold backend code (`just gen module <name>`, which also writes the module's first migration, `just gen migration <module> <name>`; `just gen help` lists every generator).
gen target *args:
    #!/usr/bin/env bash
    set -euo pipefail
    case "{{target}}" in
      docs)
        ./scripts/gen-docs.sh
        ;;
      module)
        cd apps/backend
        go run ./cmd/monacoctl gen module {{args}}
        go run ./cmd/monacoctl gen migration {{args}} init
        ;;
      *)
        cd apps/backend && go run ./cmd/monacoctl gen {{target}} {{args}}
        ;;
    esac

killports:
    #!/usr/bin/env bash
    set -euo pipefail
    # App dev ports only — Postgres stays up (use just reset db for volume wipe).
    port=8080
    if [[ -n "${API_ADDR:-}" ]]; then
      port="${API_ADDR##*:}"
    elif command -v dotenvx >/dev/null 2>&1 && [[ -f .env.local ]]; then
      addr="$(dotenvx get API_ADDR -f .env.local 2>/dev/null || true)"
      if [[ -n "$addr" ]]; then
        port="${addr##*:}"
      fi
    fi
    ./scripts/kill-listeners.sh "$port"

# Overnight QA loop: backend, host tests, app unit tests, then each sample UI test class
# one at a time on a slimmed simulator. Report lands in .logs/qa/<timestamp>/report.md.
# Examples: `just qa-night`, `just qa-night --until 07:30`, `just qa-night --skip-backend`.
qa-night *args:
    ./scripts/qa/night.sh {{args}}
