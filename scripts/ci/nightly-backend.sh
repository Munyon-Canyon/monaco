#!/usr/bin/env bash
set -uo pipefail

out="${OUT:?OUT must name a directory for logs and bench.txt}"
fuzztime="${FUZZTIME:-10m}"
sweep="${SWEEP:-20}"
mkdir -p "$out"
out="$(cd "$out" && pwd)"
cd "$(dirname "$0")/../../apps/backend"
export RAPID_NOFAILFILE=1
failed=()

step() {
  local name="$1"
  shift
  echo "::group::$name"
  if "$@" > >(tee "$out/$name.log") 2>&1; then
    echo "::endgroup::"
  else
    echo "::endgroup::"
    echo "FAILED: $name (log $out/$name.log)"
    failed+=("$name")
  fi
}

rapid_packages() {
  go list -f '{{.ImportPath}} {{join .TestImports " "}} {{join .XTestImports " "}}' ./... |
    awk '/ pgregory\.net\/rapid( |$)/ { print $1 }'
}

fuzz_all() {
  local status=0 names=() line
  while IFS= read -r line; do
    case "$line" in
      Fuzz*) names+=("$line") ;;
      ok*)
        pkg="$(awk '{ print $2 }' <<<"$line")"
        for name in ${names[@]+"${names[@]}"}; do
          echo "fuzz $pkg $name for $fuzztime"
          go test -run '^$' -fuzz "^${name}\$" -fuzztime "$fuzztime" "$pkg" || status=1
        done
        names=()
        ;;
    esac
  done < <(go test -list '^Fuzz' ./...)
  return "$status"
}

rapid=()
while IFS= read -r pkg; do
  rapid+=("$pkg")
done < <(rapid_packages)
step rapid env RAPID_CHECKS=100000 go test -timeout 60m "${rapid[@]}"
step fuzz fuzz_all
step long go test -race -shuffle=on -timeout 60m ./...
step seed-sweep go test -race -shuffle=on -short -count="$sweep" -timeout 60m ./...
step mutation go run ./cmd/monacoctl mutation --all
step bench go test -run '^$' -bench . -benchmem -count 10 -timeout 60m ./...
cp "$out/bench.log" "$out/bench.txt"
if [[ -n "${PREV_BENCH:-}" && -s "${PREV_BENCH}" ]]; then
  step benchstat ../../scripts/ci/bench-regressions.sh "$PREV_BENCH" "$out/bench.txt"
else
  echo "no previous bench.txt; benchstat comparison starts next night"
fi

if [[ "${#failed[@]}" -gt 0 ]]; then
  echo "failed suites: ${failed[*]}"
  exit 1
fi
