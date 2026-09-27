#!/usr/bin/env bash
set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  echo "error: jq is missing. Install it (brew install jq) and rerun." >&2
  exit 1
fi

cd "$(dirname "$0")/../apps/backend"
json="$(mktemp)"
cover="$(mktemp)"
trap 'rm -f "$json" "$cover"' EXIT

# rapid divides checks by 5 and steps by 2 under -short, so this lands on 100 cases and about 20 steps.
# Env vars, not -rapid.* flags: a test binary that does not link rapid rejects the flags.
export RAPID_CHECKS=500 RAPID_STEPS=40
if [[ -n "${CI:-}" ]]; then
  export RAPID_NOFAILFILE=1
fi

status=0
start="$(date +%s)"
summary='select((.Action == "output" and .Test == null and (.Output | test("^(PASS|-test\\.shuffle )") | not))
  or .Action == "build-output") | .Output'
go test -json -race -shuffle=on -short -coverpkg=./... -coverprofile="$cover" "$@" ./... | tee "$json" | jq -rj --unbuffered "$summary" || status=1

# -race makes sync.Pool drop items at random, so allocation baselines run in a second pass without it.
allocs="$(find . -name allocs_test.go -not -path '*/testdata/*' -exec dirname {} \; | sort -u | tr '\n' ' ')"
if [[ -n "$allocs" ]]; then
  # shellcheck disable=SC2086
  go test -json -short -run '^TestAllocs' $allocs | tee -a "$json" | jq -rj --unbuffered "$summary" || status=1
fi

if [[ "$status" -ne 0 ]]; then
  echo
  echo "--- output of failed tests ---"
  jq -rsj '[.[] | select(.Action == "fail") | "\(.Package) \(.Test // "")"] as $failed
    | .[] | select(.Action == "output")
    | select(if .Test == null then (.Output | startswith("-test.shuffle ")) and ("\(.Package) " | IN($failed[]))
      else "\(.Package) \(.Test)" | IN($failed[]) end)
    | .Output' "$json"
fi

report=(--from "$json" --start "$start")
if [[ -n "${CI:-}" ]]; then
  report+=(--ci)
fi
go run ./cmd/monacoctl test-report "${report[@]}" || status=1
go run ./cmd/monacoctl flows check --from "$json" || status=1
go run ./cmd/monacoctl coverage --profile "$cover" | tail -n 40 || status=1
exit "$status"
