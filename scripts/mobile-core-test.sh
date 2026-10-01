#!/usr/bin/env bash
# The one entry point for mobile-core's tests: just test mobile and ci-mobile-core.yml call it,
# and monacoctl agents check will from #965 PR 4. Runs swift test with warnings as errors and coverage,
# then holds line coverage of Sources/ to this platform's row in coverage-floor.txt.
# Extra arguments go to swift test and skip the coverage check, because a partial run
# cannot be judged. --update-floor raises this platform's floor to the measured value and
# refuses to lower it. Bash, awk and llvm-cov only: swift:6.3-noble has no python3 or jq.
# Rules: docs/architecture/ci.md#what-runs-where
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd -P)"
floor_file="$root/packages/mobile-core/coverage-floor.txt"

update=false
if [[ "${1:-}" == --update-floor ]]; then
  update=true
  shift
  (($# == 0)) || { echo "--update-floor takes no other arguments" >&2; exit 2; }
fi

if [[ "$(uname -s)" == Darwin ]]; then
  platform=darwin
  binary=.build/debug/MonacoCorePackageTests.xctest/Contents/MacOS/MonacoCorePackageTests
  llvm_cov=(xcrun llvm-cov)
else
  platform=linux
  binary=.build/debug/MonacoCorePackageTests.xctest
  llvm_cov=(llvm-cov)
fi

cd "$root/packages/mobile-core"
mkdir -p .build
status=0
swift test --force-resolved-versions -Xswiftc -warnings-as-errors --enable-code-coverage "$@" 2>&1 | tee .build/test-output.txt || status=$?
((status == 0)) || exit "$status"
(($# == 0)) || exit 0

cov_args=(report "$binary" -instr-profile .build/debug/codecov/default.profdata -ignore-filename-regex='(\.build|Tests)/')
report="$("${llvm_cov[@]}" "${cov_args[@]}")"
actual="$(awk '$1 == "TOTAL" { sub(/%$/, "", $10); print $10 }' <<<"$report")"
[[ -n "$actual" ]] || { echo "no TOTAL row in llvm-cov report" >&2; exit 1; }
floor="$(awk -v p="$platform" '$1 == p { print $2; exit }' "$floor_file" 2>/dev/null || true)"

below() { awk -v a="$1" -v b="$2" 'BEGIN { exit !(a + 0 < b + 0) }'; }

if $update; then
  if [[ -n "$floor" ]] && below "$actual" "$floor"; then
    echo "refusing to lower the floor: $platform $floor -> $actual" >&2
    exit 1
  fi
  rows="$(awk -v p="$platform" '$1 != p' "$floor_file" 2>/dev/null || true)"
  printf '%s\n%s %s\n' "$rows" "$platform" "$actual" | awk 'NF' | sort >"$floor_file"
  echo "coverage floor: $platform ${floor:-none} -> $actual"
  exit 0
fi

if [[ -z "$floor" ]]; then
  echo "no coverage floor for $platform in $floor_file (measured $actual): run scripts/mobile-core-test.sh --update-floor" >&2
  exit 1
fi
if below "$actual" "$floor"; then
  echo "coverage fell: $platform $floor -> $actual" >&2
  echo "files with the most missed lines:" >&2
  awk '$1 != "TOTAL" && $9 ~ /^[0-9]+$/ && $9 > 0 { print $9, $10, $1 }' <<<"$report" | sort -rn | head -10 >&2
  exit 1
fi
if below "$(awk -v f="$floor" 'BEGIN { print f + 0.5 }')" "$actual"; then
  echo "raise the floor: $platform $floor -> $actual (scripts/mobile-core-test.sh --update-floor)" >&2
  exit 1
fi
echo "coverage: $platform $actual (floor $floor)"
