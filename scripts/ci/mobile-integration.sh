#!/usr/bin/env bash
# Boot api, worker and fakes, then run mobile-core tests whose names contain Integration.
set -euo pipefail

root="$(cd "$(dirname "$0")/../.." && pwd)"
logdir="${RUNNER_TEMP:-/tmp}/mobile-integration"
mkdir -p "$logdir"
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
docker compose up -d --wait postgres nats

export DATABASE_URL="postgres://monaco:monaco@127.0.0.1:54322/monaco?sslmode=disable"
export NATS_URL="nats://127.0.0.1:4222"
"$root/.bin/atlas" migrate apply --dir "file://$root/apps/backend/migrations" --url "$DATABASE_URL"

(
  cd "$root/apps/backend"
  go build -o "$logdir/api" ./cmd/api
  go build -o "$logdir/worker" ./cmd/worker
  go build -o "$logdir/fakes" ./cmd/fakes
  go build -o "$logdir/monacoctl" ./cmd/monacoctl
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
  go run "$keys" "$logdir/privy-auth" "$logdir/privy-verify"
)
rm -f "$keys"

token_key="$(python3 -c 'import secrets; print(secrets.token_hex(32))')"
dev_user="$(python3 -c 'import uuid; print(uuid.uuid4())')"
export MONACO_ENV=test
export MONACO_DEV_TOKEN_KEY="$token_key"
export MONACO_DEV_USER="$dev_user"
"$logdir/monacoctl" dev token --user "$dev_user" > "$logdir/dev-token"
MONACO_DEV_TOKEN="$(cat "$logdir/dev-token")"
export MONACO_DEV_TOKEN
export MONACO_BUS_ACK_WAIT=100ms
export PRIVY_APP_ID=verify-app
export PRIVY_APP_SECRET=verify-app-secret
PRIVY_VERIFICATION_KEY="$(cat "$logdir/privy-verify")"
export PRIVY_VERIFICATION_KEY
export PRIVY_AUTHORIZATION_KEY_ID=fixture-key-quorum
PRIVY_AUTHORIZATION_PRIVATE_KEY="$(cat "$logdir/privy-auth")"
export PRIVY_AUTHORIZATION_PRIVATE_KEY

FAKES_ADDR=127.0.0.1:0 "$logdir/fakes" > "$logdir/fakes.log" 2>&1 &
pids+=("$!")
fakes_addr="$(wait_addr "$logdir/fakes.log")"
fakes_url="http://${fakes_addr}"
export MONACO_JUPITER_SWAP_BASE_URL="${fakes_url}/jupiter/swap/v2"
export MONACO_JUPITER_PRICE_BASE_URL="${fakes_url}/jupiter/price/v3"
export XSTOCKS_BASE_URL="${fakes_url}/xstocks"
export SOLANA_RPC_URL="${fakes_url}/rpc/"
export PRIVY_BASE_URL="${fakes_url}/privy"

MONACO_HTTP_ADDR=127.0.0.1:0 "$logdir/api" > "$logdir/api.log" 2>&1 &
pids+=("$!")
MONACO_WORKER_HEALTH_ADDR=127.0.0.1:0 "$logdir/worker" > "$logdir/worker.log" 2>&1 &
pids+=("$!")

api_addr="$(wait_addr "$logdir/api.log")"
worker_addr="$(wait_addr "$logdir/worker.log")"
export MONACO_API_URL="http://${api_addr}"

ready=0
tries=0
while [[ "$tries" -lt 30 ]]; do
  if curl -sf "$MONACO_API_URL/healthz" >/dev/null && curl -sf "http://${worker_addr}/healthz" >/dev/null; then
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
  -e MONACO_API_URL -e MONACO_DEV_TOKEN -e MONACO_DEV_USER \
  swift:6.3-noble swift test --filter Integration \
  > "$logdir/swift.log" 2>&1
swift_status=$?
set -e

pass_pat="$(printf '%s' "Test Case '-\\[.*Integration.*\\]' (passed|failed)")"
skip_pat="$(printf '%s' "Test Case '-\\[.*Integration.*\\]' skipped")"
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
