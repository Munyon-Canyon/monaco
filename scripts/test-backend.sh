#!/usr/bin/env bash
set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  echo "error: jq is missing. Install it (brew install jq) and rerun." >&2
  exit 1
fi

cd "$(dirname "$0")/../apps/backend"
json="$(mktemp)"
trap 'rm -f "$json"' EXIT

status=0
go test -json "$@" ./... | tee "$json" |
  jq -rj --unbuffered 'select((.Action == "output" and .Test == null and (.Output | test("^(PASS|-test\\.shuffle )") | not))
    or .Action == "build-output") | .Output' || status=1

if [[ "$status" -ne 0 ]]; then
  echo
  echo "--- output of failed tests ---"
  jq -rsj '[.[] | select(.Action == "fail") | "\(.Package) \(.Test // "")"] as $failed
    | .[] | select(.Action == "output")
    | select(if .Test == null then (.Output | startswith("-test.shuffle ")) and ("\(.Package) " | IN($failed[]))
      else "\(.Package) \(.Test)" | IN($failed[]) end)
    | .Output' "$json"
fi

go run ./cmd/monacoctl flows check --from "$json" || status=1
exit "$status"
