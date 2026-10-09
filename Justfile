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
    for f in .env.production .env.staging; do
      if [[ -f "$f" ]]; then
        dotenvx encrypt -f "$f"
      fi
    done

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
    for f in .env.production .env.staging; do
      if [[ -f "$f" ]]; then
        dotenvx decrypt -f "$f"
      fi
    done

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

# just run [backend|mobile] [--env local|staging]. staging runs only the app, against the Render staging API.
run *args:
    #!/usr/bin/env bash
    set -euo pipefail
    app=""
    env_name=local
    set -- {{args}}
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --env) env_name="${2:-}"; shift 2 || { echo "error: --env needs local or staging" >&2; exit 1; } ;;
        --env=*) env_name="${1#--env=}"; shift ;;
        backend|mobile) app="$1"; shift ;;
        *) echo "error: unknown argument '$1' (use backend, mobile, --env local|staging)" >&2; exit 1 ;;
      esac
    done
    case "$env_name" in
      local) ;;
      staging)
        if [[ "$app" == backend ]]; then
          echo "error: the staging backend runs on Render (docs/how-to/deploy-staging.md); use just run mobile --env staging" >&2
          exit 1
        fi
        app=mobile
        ;;
      *) echo "error: --env must be local or staging, not '$env_name'" >&2; exit 1 ;;
    esac
    if [[ -z "$app" ]]; then
      # Bash starts a background job with SIGINT ignored, so Ctrl-C never reaches the
      # backend. Stop it by name on every exit instead: Ctrl-C, a failed mobile build, or
      # the backend exiting on its own.
      trap 'just stop backend >/dev/null 2>&1 || true' EXIT
      trap 'exit 130' INT
      trap 'exit 143' TERM
      source ./scripts/run-with-logs.sh
      monaco_init_logs
      just run backend &
      backend_pid=$!
      just run mobile
      monaco_step "ready: app on the simulator, backend on :8080. Ctrl+C stops everything."
      wait "$backend_pid"
      exit 0
    fi
    case "$app" in
      backend)
        source ./scripts/run-with-logs.sh
        monaco_step "backend: building api, worker and monacoctl"
        just build backend
        ./scripts/require-docker.sh
        monaco_step "backend: starting postgres and nats"
        docker compose up -d --wait postgres nats
        monaco_init_logs
        monaco_step "backend: starting api and worker"
        # Set before anything can fail or block, so a failing step never orphans api or worker. By
        # name, not by pid: the pids are the dotenvx wrappers, and the INT trap never fires when
        # `just run` started this as a background job (SIGINT ignored). The fund page is stopped only
        # when this run started it, and only this checkout's Vite.
        ready_pid=""
        fund_pid=""
        fund_vite="^(node )?${PWD}/apps/web/node_modules/.bin/vite( |$)"
        trap 'if [[ -n "$ready_pid" ]]; then kill "$ready_pid" 2>/dev/null || true; fi; pkill -TERM -f "^${PWD}/bin/(api|worker)$" || true
          if [[ -n "$fund_pid" ]]; then kill "$fund_pid" 2>/dev/null || true; pkill -TERM -f "$fund_vite" || true; fi' INT TERM EXIT
        # MONACO_FUND_PAGE_PORT is ours, and the backend refuses unknown MONACO_ variables at boot.
        env -u MONACO_LOG_DIR -u MONACO_DOTENVX -u MONACO_FUND_PAGE_PORT {{_dotenvx}} "$PWD/bin/api" > >(tee -a "${MONACO_LOG_DIR}/api.log") 2>&1 &
        api_pid=$!
        env -u MONACO_LOG_DIR -u MONACO_DOTENVX -u MONACO_FUND_PAGE_PORT {{_dotenvx}} "$PWD/bin/worker" > >(tee -a "${MONACO_LOG_DIR}/worker.log") 2>&1 &
        worker_pid=$!
        api_port="${MONACO_HTTP_ADDR:-:8080}"; api_port="${api_port##*:}"
        worker_port="${MONACO_WORKER_HEALTH_ADDR:-:8081}"; worker_port="${worker_port##*:}"
        # Card deposits open the fund page on this port (apps/web/README.md). Skipped when the port
        # is taken, Node is missing, the install fails, or MONACO_FUND_PAGE_PORT=off (journey slots);
        # the backend starts either way.
        fund_port="${MONACO_FUND_PAGE_PORT:-5173}"
        if [[ "$fund_port" == "off" ]]; then
          :
        elif lsof -nP -iTCP:"$fund_port" -sTCP:LISTEN -t >/dev/null 2>&1; then
          monaco_step "fund page: port ${fund_port} is taken; not started"
        elif ! command -v npx >/dev/null 2>&1; then
          echo "fund page not started: Node is not installed; card deposits will not open"
        elif [[ ! -d apps/web/node_modules ]] && ! { monaco_step "fund page: installing apps/web dependencies"; (cd apps/web && npm ci) >>"${MONACO_LOG_DIR}/fund.log" 2>&1; }; then
          echo "fund page not started: npm ci failed; see ${MONACO_LOG_DIR}/fund.log"
        else
          monaco_step "fund page: starting on http://localhost:${fund_port}/fund"
          env -u MONACO_LOG_DIR -u MONACO_DOTENVX -u MONACO_FUND_PAGE_PORT {{_dotenvx}} bash -c 'cd apps/web && VITE_MONACO_API_URL="http://localhost:$1" VITE_PRIVY_APP_ID="$PRIVY_APP_ID" VITE_PRIVY_ENV=sandbox exec npx vite --port "$2" --strictPort' _ "$api_port" "$fund_port" > >(tee -a "${MONACO_LOG_DIR}/fund.log") 2>&1 &
          fund_pid=$!
        fi
        (
          for _ in $(seq 1 120); do
            if curl -fsS -o /dev/null "http://localhost:${api_port}/healthz" 2>/dev/null &&
              curl -fsS -o /dev/null "http://localhost:${worker_port}/healthz" 2>/dev/null; then
              monaco_step "backend: running (api http://localhost:${api_port}, worker :${worker_port})"
              exit 0
            fi
            sleep 0.5
          done
          monaco_step "backend: not healthy after 60s; see ${MONACO_LOG_DIR}/api.log and worker.log"
        ) &
        ready_pid=$!
        wait "$api_pid" "$worker_pid"
        ;;
      mobile)
        if [[ "${MONACO_DOTENVX:-}" != "1" ]]; then
          exec {{_dotenvx}} env MONACO_DOTENVX=1 just run mobile --env "$env_name"
        fi
        if [[ ! -d apps/mobile ]]; then
          echo "error: apps/mobile is not scaffolded yet (M0-T4)."
          exit 1
        fi
        # Privy: xcconfig + SIMCTL_CHILD_* via with-ios-privy-env, then scripts/ios-sim
        source ./scripts/run-with-logs.sh
        monaco_init_logs
        if [[ "$env_name" == staging ]]; then
          if [[ "${MONACO_STAGING_API_BASE_URL:-}" != https://* ]]; then
            echo "error: MONACO_STAGING_API_BASE_URL in .env.local must be the https staging API" >&2
            exit 1
          fi
          export MONACO_ENVIRONMENT=staging MONACO_API_BASE_URL="$MONACO_STAGING_API_BASE_URL"
          monaco_step "mobile: Debug build against staging ${MONACO_API_BASE_URL}"
        fi
        ./scripts/ios-sim 2>&1 | tee -a "${MONACO_LOG_DIR}/mobile.log"
        ;;
      *)
        echo "error: unknown app '$app' (use backend or mobile)"
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
        # Only this checkout's fund page: another checkout's Vite, or an unrelated app, may hold the port.
        pkill -TERM -f "^(node )?${PWD}/apps/web/node_modules/.bin/vite( |$)" || true
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
        ./scripts/qa/xcode-lock.sh xcode xcodebuild -project apps/mobile/Monaco.xcodeproj -scheme Monaco \
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

# Apply pending migrations to the .env.local database, print its revision, then apply the NATS stream config. `just run backend` does neither.
migrate target:
    #!/usr/bin/env bash
    set -euo pipefail
    case "{{target}}" in
      db)
        mkdir -p bin
        (cd apps/backend && go build -o ../../bin/ ./cmd/monacoctl)
        docker compose up -d --wait postgres nats
        {{_dotenvx}} "$PWD/bin/monacoctl" migrate apply
        {{_dotenvx}} "$PWD/bin/monacoctl" migrate status
        {{_dotenvx}} "$PWD/bin/monacoctl" bus apply
        ;;
      *)
        echo "error: unknown target '{{target}}' (use db)"
        exit 1
        ;;
    esac

# Print the relayer's pubkey and its SOL balance from the .env.local key and RPC (`just relayer balance`).
relayer target:
    #!/usr/bin/env bash
    set -euo pipefail
    case "{{target}}" in
      balance)
        mkdir -p bin
        (cd apps/backend && go build -o ../../bin/ ./cmd/monacoctl)
        {{_dotenvx}} "$PWD/bin/monacoctl" relayer balance
        ;;
      *)
        echo "error: unknown target '{{target}}' (use balance)"
        exit 1
        ;;
    esac

# Regenerate every checked-in generated file (`just gen docs`, which runs `go generate ./...`) or scaffold backend code (`just gen module <name>`, which also writes the module's first migration, `just gen migration <module> <name>`; `just gen help` lists every generator).
gen target *args:
    #!/usr/bin/env bash
    set -euo pipefail
    case "{{target}}" in
      docs)
        cd apps/backend && go generate ./...
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
    # App dev ports only (api, worker health and the fund page); Postgres stays up (just reset db wipes it).
    port_of() {
      local key=$1 addr="${!1:-}"
      if [[ -z "$addr" ]] && command -v dotenvx >/dev/null 2>&1 && [[ -f .env.local ]]; then
        addr="$(dotenvx get "$key" -f .env.local 2>/dev/null || true)"
      fi
      if [[ -n "$addr" ]]; then echo "${addr##*:}"; else echo "$2"; fi
    }
    ./scripts/kill-listeners.sh "$(port_of MONACO_HTTP_ADDR 8080)" "$(port_of MONACO_WORKER_HEALTH_ADDR 8081)" $([[ "${MONACO_FUND_PAGE_PORT:-}" == "off" ]] || echo "${MONACO_FUND_PAGE_PORT:-5173}")

# Overnight QA loop: backend, host tests, app unit tests, then each sample UI test class
# one at a time on a slimmed simulator. Report lands in .logs/qa/<timestamp>/report.md.
# Examples: `just qa-night`, `just qa-night --until 07:30`, `just qa-night --skip-backend`.
qa-night *args:
    ./scripts/qa/night.sh {{args}}
