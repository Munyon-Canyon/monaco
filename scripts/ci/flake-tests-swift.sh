#!/usr/bin/env bash
# Reruns the changed mobile-core test files 10 times on the already-built package, the Swift
# twin of flake-tests.sh. For each changed or added *.swift under packages/mobile-core/Tests/ it
# collects the XCTestCase subclasses and @Suite types the file declares, prints one
# swift test --filter '<Module>\.(<A>|<B>)' per test module, and runs it 10 times in a row.
# The first red run fails the script and names its iteration. A file that declares neither
# (a helper, or free @Test functions) adds no filter. Bash, awk and git only: swift:6.3-noble has
# no python3 or jq. Rules: docs/architecture/ci.md#what-runs-where
set -euo pipefail

runs=10
print_only=0
root=""
base=""
files=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --print) print_only=1; shift ;;
    --root) root="$2"; shift 2 ;;
    --base) base="$2"; shift 2 ;;
    --) shift; files+=("$@"); break ;;
    *) files+=("$1"); shift ;;
  esac
done

if [[ -z "$root" ]]; then
  root="$(cd "$(dirname "$0")/../.." && pwd)"
fi
cd "$root"

if [[ ${#files[@]} -eq 0 && -n "$base" ]]; then
  while IFS= read -r f; do
    [[ -n "$f" ]] && files+=("$f")
  done < <(git diff --name-only --diff-filter=d "$base"...HEAD -- 'packages/mobile-core/Tests/*.swift')
fi

if [[ ${#files[@]} -eq 0 ]]; then
  exit 0
fi

tests_dir="packages/mobile-core/Tests/"
declared=""
for f in "${files[@]}"; do
  rel="${f#./}"
  rel="${rel#"$root"/}"
  [[ "$rel" == "$tests_dir"*/*.swift && -f "$rel" ]] || continue
  module="${rel#"$tests_dir"}"
  module="${module%%/*}"
  declared+="$(awk -v module="$module" -f /dev/stdin "$rel" <<'AWK'
function name_after(s,    n) {
  if (!match(s, /(^|[^A-Za-z0-9_])(struct|class|enum|actor)[ \t]+[A-Za-z_][A-Za-z0-9_]*/)) return ""
  n = substr(s, RSTART, RLENGTH)
  sub(/^[^A-Za-z]*(struct|class|enum|actor)[ \t]+/, "", n)
  return n
}
/^[ \t]*\/\// { next }
match($0, /(^|[^A-Za-z0-9_])class[ \t]+[A-Za-z_][A-Za-z0-9_]*[ \t]*:[ \t]*XCTestCase([^A-Za-z0-9_]|$)/) {
  n = substr($0, RSTART, RLENGTH)
  sub(/^[^A-Za-z]*class[ \t]+/, "", n)
  sub(/[^A-Za-z0-9_].*$/, "", n)
  print module "\t" n
}
{
  rest = $0
  if (index(rest, "@Suite") > 0) { pending = 1; rest = substr(rest, index(rest, "@Suite") + 6) }
  if (pending) {
    n = name_after(rest)
    if (n != "") { print module "\t" n; pending = 0 }
    else if (rest !~ /^[ \t]*(@|$)/ && rest !~ /^[ \t]*\(/ && index($0, "@Suite") == 0) pending = 0
  }
}
AWK
)"$'\n'
done

cmds="$(awk -F '\t' -v q="'" 'NF == 2 && !seen[$1 SUBSEP $2]++ { if (!($1 in names)) order[++n] = $1; names[$1] = names[$1] (names[$1] == "" ? "" : "|") $2 } END { for (i = 1; i <= n; i++) printf "swift test --filter %s%s\\.(%s)%s\n", q, order[i], names[order[i]], q }' <<<"$declared")"

if [[ -z "$cmds" ]]; then
  exit 0
fi
if [[ "$print_only" -eq 1 ]]; then
  printf '%s\n' "$cmds"
  exit 0
fi
cd "$root/packages/mobile-core"
while IFS= read -r cmd; do
  [[ -n "$cmd" ]] || continue
  for ((i = 1; i <= runs; i++)); do
    echo "flake run $i of $runs: $cmd"
    eval "$cmd" || { echo "flake: red on run $i of $runs: $cmd" >&2; exit 1; }
  done
done <<<"$cmds"
