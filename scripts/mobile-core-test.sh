#!/usr/bin/env bash
# The one entry point for mobile-core's tests: just test mobile and ci-mobile-core.yml call it,
# and monacoctl agents check will from #965 PR 4. Runs swift test with warnings as errors and coverage,
# then holds every single test to a 2 s budget and line coverage of Sources/ to this
# platform's row in coverage-floor.txt. Extra arguments go to swift test and skip both
# checks, because a partial run cannot be judged. Coverage below the floor fails. Coverage more than 0.5 above it
# never fails and never edits the file: the run prints "coverage rose" (ci-mobile-core.yml turns it into a warning).
# Only a small PR of its own commits a raise, with --update-floor, which raises the row at any margin and refuses to
# lower it, so feature PRs never conflict on coverage-floor.txt. Bash, awk and llvm-cov only: swift:6.3-noble has no python3 or jq.
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

if [[ "$(uname -s)" == Darwin && -z "${GITHUB_ACTIONS:-}" && "${MONACO_SWIFTPM_LOCKED:-}" != 1 ]]; then
  # Concurrent cold swift builds use every core each and overload the machine, so queue for a swiftpm slot.
  if [[ "$update" == true ]]; then set -- --update-floor; fi
  MONACO_SWIFTPM_LOCKED=1 exec "$root/scripts/qa/xcode-lock.sh" swiftpm "$0" "$@"
fi

if [[ "$(uname -s)" == Darwin ]]; then
  platform=darwin
  llvm_cov=(xcrun llvm-cov)
else
  platform=linux
  llvm_cov=(llvm-cov)
fi

# Only stdout is teed into the file the timing parser reads. stderr goes straight to the
# caller: a SwiftPM warning printed while tests run would otherwise land inside a buffered
# "Test Case ... passed" line and leave that case untimed. Both XCTest and swift-testing
# report on stdout. No --build-system flag: each toolchain's default is used, and the
# coverage step below finds the test binaries wherever that build system put them.
cd "$root/packages/mobile-core"
mkdir -p .build
status=0
swift test --force-resolved-versions -Xswiftc -warnings-as-errors --enable-code-coverage "$@" | tee .build/test-output.txt || status=$?
((status == 0)) || exit "$status"
(($# == 0)) || exit 0

budget=2
# XCTest starts a case as "Test Case '...' started at ..." on Linux and
# "Test Case '...' started." on macOS (only suite lines say "started at" there).
# It ends a line with "(<n> seconds)" and macOS adds a period. swift-testing ends with
# "passed after <n> seconds." Suite and run summaries are not tests. Every XCTest case that
# started must have a timing, so a changed output format fails here instead of passing
# unjudged. There is no allowlist: a slow test gets faster.
timings="$(awk -v q="'" -f /dev/stdin .build/test-output.txt <<'AWK'
  $0 ~ "^Test Case " q ".*" q " started( at |\\.$)" { started++; next }
  $0 ~ "^Test Case " q ".*" q " [a-z]+ \\([0-9.]+ seconds\\)\\.?$" {
    timed++
    name = $0; sub("^Test Case " q, "", name); sub(q " [a-z]+ \\([0-9.]+ seconds\\)\\.?$", "", name)
    secs = $0; sub(/ seconds\)\.?$/, "", secs); sub(/.*\(/, "", secs)
    print secs "\t" name
    next
  }
  / passed after [0-9.]+ seconds\.$/ && /^[^A-Za-z]*Test / {
    name = $0; sub(/^[^A-Za-z]*Test /, "", name)
    if (name ~ /^run with /) next
    secs = name; sub(/ seconds\.$/, "", secs); sub(/.* passed after /, "", secs)
    sub(/ passed after [0-9.]+ seconds\.$/, "", name)
    print secs "\t" name
  }
  END {
    if (started != timed) {
      printf "unparsed test timings: %d XCTest cases started, %d timed\n", started, timed > "/dev/stderr"
      exit 1
    }
  }
AWK
)"
[[ -n "$timings" ]] || { echo "no test timings in .build/test-output.txt" >&2; exit 1; }
slow="$(awk -F '\t' -v b="$budget" '$1 + 0 > b { printf "slow test: %s %ss (budget %s s)\n", $2, $1, b }' <<<"$timings")"
if [[ -n "$slow" ]]; then
  echo "$slow" >&2
  exit 1
fi
awk -F '\t' '$1 + 0 > m { m = $1 + 0; n = $2 } END { printf "slowest test: %s %ss (budget %s s)\n", n, m, b }' b="$budget" <<<"$timings"

# The native build system (Linux) links one MonacoCorePackageTests.xctest; Swift Build
# (macOS, Swift 6.4 and later) links one bundle per test target. Every bundle next to the
# codecov directory is passed to llvm-cov, which merges their coverage by source file.
codecov="$(dirname "$(swift test --show-codecov-path)")"
objects=()
for bundle in "$(dirname "$codecov")"/*.xctest; do
  if [[ -d "$bundle" ]]; then
    objects+=(-object "$bundle/Contents/MacOS/$(basename "$bundle" .xctest)")
  elif [[ -f "$bundle" ]]; then
    objects+=(-object "$bundle")
  fi
done
((${#objects[@]} > 0)) || { echo "no test binaries next to $codecov" >&2; exit 1; }
cov_args=(report "${objects[@]:1}" -instr-profile "$codecov/default.profdata" -ignore-filename-regex='(\.build|Tests)/')
report="$("${llvm_cov[@]}" "${cov_args[@]}")"
actual="$(awk '$1 == "TOTAL" { sub(/%$/, "", $10); print $10 }' <<<"$report")"
[[ -n "$actual" ]] || { echo "no TOTAL row in llvm-cov report" >&2; exit 1; }
floor="$(awk -v p="$platform" '$1 == p { print $2; exit }' "$floor_file" 2>/dev/null || true)"

below() { awk -v a="$1" -v b="$2" 'BEGIN { exit !(a + 0 < b + 0) }'; }

write_floor() {
  rows="$(awk -v p="$platform" '$1 != p' "$floor_file" 2>/dev/null || true)"
  printf '%s\n%s %s\n' "$rows" "$platform" "$actual" | awk 'NF' | sort >"$floor_file"
}

if $update; then
  if [[ -n "$floor" ]] && below "$actual" "$floor"; then
    echo "refusing to lower the floor: $platform $floor -> $actual" >&2
    exit 1
  fi
  write_floor
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
  echo "coverage rose: $platform $floor -> $actual (do not commit it in a feature PR: raise the floor in its own PR with scripts/mobile-core-test.sh --update-floor)"
fi
echo "coverage: $platform $actual (floor $floor)"
