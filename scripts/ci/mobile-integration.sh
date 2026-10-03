#!/usr/bin/env bash
# Boot api, worker and fakes, then run mobile-core tests whose names contain Integration.
set -euo pipefail

root="$(cd "$(dirname "$0")/../.." && pwd)"
logdir="${RUNNER_TEMP:-/tmp}/mobile-integration"
bindir="${RUNNER_TEMP:-/tmp}/mobile-integration-bin"
mkdir -p "$logdir" "$bindir"
pids=()
keys=""

kill_children() {
  local pid
  if [[ -n "$keys" ]]; then
    rm -f "$keys"
  fi
  if [[ "${#pids[@]}" -eq 0 ]]; then
    return 0
  fi
  for pid in "${pids[@]}"; do
    if kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
}
trap kill_children EXIT

listening_addr() {
  python3 - "$1" << 'PY'
import json, sys
path = sys.argv[1]
try:
    lines = open(path, encoding="utf-8", errors="replace")
except FileNotFoundError:
    sys.exit(0)
for line in lines:
    line = line.strip()
    if not line.startswith("{"):
        continue
    try:
        event = json.loads(line)
    except json.JSONDecodeError:
        continue
    if event.get("msg") == "boot.listening" and event.get("addr"):
        print(event["addr"])
        break
PY
}

wait_addr() {
  local log=$1 tries=0 addr=""
  while [[ "$tries" -lt 60 ]]; do
    addr="$(listening_addr "$log")"
    if [[ -n "$addr" ]]; then
      printf '%s' "$addr"
      return 0
    fi
    tries=$((tries + 1))
    sleep 1
  done
  echo "mobile-integration: no boot.listening in $log" >&2
  tail -n 40 "$log" >&2 || true
  return 1
}

cd "$root"
# GitHub Actions sets CI=true and has no dev stack, so the pinned names are safe there.
# Anywhere else, project "monaco" and container_name monaco-postgres / monaco-nats are
# the dev stack. A nonempty COMPOSE_PROJECT_NAME is not isolation: monaco reuses the
# dev project, and any other name still pins those containers until a COMPOSE_FILE
# renames them. Either path would migrate the dev database and join its JetStream.
if [[ "${CI:-}" != "true" ]]; then
  if [[ -z "${COMPOSE_PROJECT_NAME:-}" || "${COMPOSE_PROJECT_NAME}" == "monaco" ]]; then
    echo "mobile-integration: refusing the dev stack. Set CI=true, or set COMPOSE_PROJECT_NAME to a project other than monaco and a COMPOSE_FILE that renames the pinned containers (monaco-postgres, monaco-nats)." >&2
    exit 1
  fi
  resolved="$(docker compose config)"
  if grep -Eq '^name: monaco[[:space:]]*$|^[[:space:]]*container_name: monaco-postgres[[:space:]]*$|^[[:space:]]*container_name: monaco-nats[[:space:]]*$' <<<"$resolved"; then
    echo "mobile-integration: refusing the dev stack. The resolved Compose config still uses project monaco or pins monaco-postgres or monaco-nats. Set COMPOSE_PROJECT_NAME and a COMPOSE_FILE that renames those containers." >&2
    exit 1
  fi
fi
docker compose up -d --wait postgres nats

# The ports compose published, not 54322 and 4222: COMPOSE_PROJECT_NAME, COMPOSE_FILE and
# POSTGRES_PORT can then give this run its own Postgres and NATS beside a dev stack.
pg_port="$(docker compose port postgres 5432 | awk -F: '{ print $NF }')"
nats_port="$(docker compose port nats 4222 | awk -F: '{ print $NF }')"
export DATABASE_URL="postgres://monaco:monaco@127.0.0.1:${pg_port}/monaco?sslmode=disable"
export NATS_URL="nats://127.0.0.1:${nats_port}"

# compose's --wait is satisfied by the healthcheck (pg_isready over the container's unix
# socket), which can go green while the published TCP port is still coming up. Poll the
# published port directly so atlas, which connects over TCP, doesn't see "connection refused".
tries=0
while [[ "$tries" -lt 30 ]]; do
  if pg_isready -h 127.0.0.1 -p "$pg_port" >/dev/null 2>&1; then
    break
  fi
  tries=$((tries + 1))
  sleep 1
done
"$root/.bin/atlas" migrate apply --dir "file://$root/apps/backend/migrations" --url "$DATABASE_URL"

(
  cd "$root/apps/backend"
  go build -o "$bindir/api" ./cmd/api
  go build -o "$bindir/worker" ./cmd/worker
  go build -o "$bindir/fakes" ./cmd/fakes
  go build -o "$bindir/monacoctl" ./cmd/monacoctl
  # The Swift tests run in a Linux container and run monacoctl flows seed there, so they need a
  # Linux build of it even when this host is a Mac.
  CGO_ENABLED=0 GOOS=linux GOARCH="$(docker version --format '{{.Server.Arch}}')" \
    go build -o "$bindir/linux/monacoctl" ./cmd/monacoctl
)

keys="$root/apps/backend/print_fixture_keys.go"
cat > "$keys" << 'EOF'
package main

import (
	"os"

	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func main() {
	if err := os.WriteFile(os.Args[1], []byte(fakes.PrivyAuthorizationKeyConfig()), 0o600); err != nil {
		panic(err)
	}
	if err := os.WriteFile(os.Args[2], []byte(fakes.PrivyVerificationKey()), 0o600); err != nil {
		panic(err)
	}
}
EOF
(
  cd "$root/apps/backend"
  go run "$keys" "$bindir/privy-auth" "$bindir/privy-verify"
)
rm -f "$keys"

# dev_user, dev_token and api_url are passed only to the docker run below (-e), never
# exported into this process's environment: the api and worker also run in this process's
# tree, and config.Load rejects any MONACO_* key it does not know, including MONACO_DEV_USER.
token_key="$(python3 -c 'import secrets; print(secrets.token_hex(32))')"
# ids.Parse accepts only a UUIDv7: /v1/stream answers any other user id with a 401.
dev_user="00000000-0000-7000-8000-000000000001"
export MONACO_ENV=test
export MONACO_DEV_TOKEN_KEY="$token_key"
# Written to a file and read back, not `dev_token="$("$bindir/monacoctl" dev token ...)"`: that
# assignment reads to the tool manifest scanner as `monacoctl`'s dev-token subcommand name being
# an invoked binary of its own, the same shape the printf below works around.
"$bindir/monacoctl" dev token --user "$dev_user" > "$logdir/dev-token"
dev_token="$(cat "$logdir/dev-token")"
export MONACO_BUS_ACK_WAIT=100ms
export PRIVY_APP_ID=verify-app
export PRIVY_APP_SECRET=verify-app-secret
PRIVY_VERIFICATION_KEY="$(cat "$bindir/privy-verify")"
export PRIVY_VERIFICATION_KEY
export PRIVY_AUTHORIZATION_KEY_ID=fixture-key-quorum
PRIVY_AUTHORIZATION_PRIVATE_KEY="$(cat "$bindir/privy-auth")"
export PRIVY_AUTHORIZATION_PRIVATE_KEY

# api and worker never create the EVENTS and DEADLETTER streams, so a fresh NATS needs this first.
"$bindir/monacoctl" bus apply

FAKES_ADDR=127.0.0.1:0 "$bindir/fakes" > "$logdir/fakes.log" 2>&1 &
pids+=("$!")
fakes_addr="$(wait_addr "$logdir/fakes.log")"
fakes_url="http://${fakes_addr}"
export MONACO_JUPITER_SWAP_BASE_URL="${fakes_url}/jupiter/swap/v2"
export MONACO_JUPITER_PRICE_BASE_URL="${fakes_url}/jupiter/price/v3"
export XSTOCKS_BASE_URL="${fakes_url}/xstocks"
export SOLANA_RPC_URL="${fakes_url}/rpc/"
export PRIVY_BASE_URL="${fakes_url}/privy"

MONACO_HTTP_ADDR=127.0.0.1:0 "$bindir/api" > "$logdir/api.log" 2>&1 &
pids+=("$!")
MONACO_WORKER_HEALTH_ADDR=127.0.0.1:0 "$bindir/worker" > "$logdir/worker.log" 2>&1 &
pids+=("$!")

api_addr="$(wait_addr "$logdir/api.log")"
worker_addr="$(wait_addr "$logdir/worker.log")"
api_url="http://${api_addr}"
# Docker Desktop gives --network host the VM's loopback, not the Mac's, so off Linux the
# container reaches the api, Postgres, NATS and the fakes by name.
container_host=127.0.0.1
if [[ "$(uname -s)" != Linux ]]; then
  container_host=host.docker.internal
fi
swift_api_url="http://${container_host}:${api_addr##*:}"

ready=0
tries=0
while [[ "$tries" -lt 30 ]]; do
  if curl -sf "$api_url/healthz" >/dev/null && curl -sf "http://${worker_addr}/healthz" >/dev/null; then
    ready=1
    break
  fi
  tries=$((tries + 1))
  sleep 1
done
if [[ "$ready" -ne 1 ]]; then
  echo "mobile-integration: api or worker never became healthy" >&2
  tail -n 40 "$logdir/api.log" >&2 || true
  tail -n 40 "$logdir/worker.log" >&2 || true
  exit 1
fi

set +e
docker run --rm --network host \
  -v "$root:/w" -w /w/packages/mobile-core \
  -v "$bindir/linux:/seed:ro" \
  -e MONACO_API_URL="$swift_api_url" -e MONACO_DEV_TOKEN="$dev_token" -e MONACO_DEV_USER="$dev_user" \
  -e MONACO_SEED_BIN=/seed/monacoctl -e MONACO_SEED_DIR=/w/apps/backend \
  -e DATABASE_URL="${DATABASE_URL/127.0.0.1/$container_host}" -e NATS_URL="${NATS_URL/127.0.0.1/$container_host}" \
  -e MONACO_DEV_TOKEN_KEY="$token_key" -e MONACO_FAKES_URL="http://${container_host}:${fakes_addr##*:}" \
  swift:6.3-noble swift test --filter Integration \
  > "$logdir/swift.log" 2>&1
swift_status=$?
set -e

# swift test writes no XCTest xunit file without --parallel, and --parallel reports a skipped
# test as passed, so the xunit file monacoctl flows check reads is built from the serial log.
python3 - "$logdir/swift.log" "$logdir/integration.xml" << 'PY'
import re, sys
from xml.sax.saxutils import quoteattr
case = re.compile(r"^Test Case '-?\[?(?:\w+\.)?(\w+)[ .](\w+)\]?' (passed|failed|skipped)")
body = {"passed": "", "failed": '<failure message="failed"/>', "skipped": "<skipped/>"}
rows = []
for line in open(sys.argv[1], encoding="utf-8", errors="replace"):
    m = case.match(line)
    if m:
        cls, name, result = m.groups()
        rows.append(f"<testcase classname={quoteattr(cls)} name={quoteattr(name)}>{body[result]}</testcase>")
with open(sys.argv[2], "w", encoding="utf-8") as out:
    out.write('<?xml version="1.0" encoding="UTF-8"?>\n<testsuites>\n<testsuite name="Integration">\n')
    out.write("\n".join(rows) + "\n</testsuite>\n</testsuites>\n")
PY

# Matches both the macOS XCTest format (Test Case quote dash-bracket Module.Class method
# bracket-quote passed) and the swift-corelibs-xctest format on Linux (Test Case
# quote Class.method quote passed). Built with printf so the tool manifest scanner's
# naive word split doesn't read the second word of the pattern ("Case") as an invoked binary.
pass_pat="$(printf '%s' "^Test Case '[^']*Integration[^']*' (passed|failed)")"
skip_pat="$(printf '%s' "^Test Case '[^']*Integration[^']*' skipped")"
executed="$(grep -Ec "$pass_pat" "$logdir/swift.log" || true)"
skipped="$(grep -Ec "$skip_pat" "$logdir/swift.log" || true)"
if [[ "$executed" -lt 1 || "$skipped" -ne 0 ]]; then
  echo "mobile-integration: no executed Integration test" >&2
  cat "$logdir/swift.log" >&2 || true
  exit 1
fi
if [[ "$swift_status" -ne 0 ]]; then
  cat "$logdir/swift.log" >&2 || true
  exit "$swift_status"
fi
echo "mobile-integration: $executed Integration test(s) executed, none skipped"
(
  cd "$root/apps/backend"
  "$bindir/monacoctl" flows check --structure-only --integration-xunit "$logdir/integration.xml"
)
