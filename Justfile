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
        echo "apps/backend: new module not scaffolded yet"
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
        echo "apps/backend: new module not scaffolded yet"
        if [[ "${SKIP_SCRIPTS_TESTS:-}" != "1" && -f scripts/go.mod ]]; then
          (cd scripts && go test -short ./...)
        fi
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
        # Host unit tests only (swift test on macOS). iOS sim UI tests stay on just build/run mobile.
        (cd packages/mobile-core && swift test)
        ;;
      *)
        echo "error: unknown app '{{app}}' (use backend or mobile)"
        exit 1
        ;;
    esac

run *app:
    #!/usr/bin/env bash
    set -euo pipefail
    if [[ -z "{{app}}" ]]; then
      just run backend
      just run mobile
      exit 0
    fi
    case "{{app}}" in
      backend)
        echo "apps/backend: new module not scaffolded yet"
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
        echo "apps/backend: new module not scaffolded yet"
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
      ./scripts/reset-db.sh
      exit 0
    fi
    case "{{target}}" in
      backend)
        echo "apps/backend: new module not scaffolded yet"
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
