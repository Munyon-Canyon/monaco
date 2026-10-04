#!/usr/bin/env bash
set -euo pipefail

version=0.65.0
image="ghcr.io/realm/swiftlint:$version"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$root"
trees=(apps/mobile packages/mobile-core)

usage() {
  echo "usage: scripts/swiftlint-ratchet.sh [--base <rev>] [path...]" >&2
  exit 64
}

base=""
paths=()
while [[ $# -gt 0 ]]; do
  if [[ "$1" == --base && $# -ge 2 ]]; then
    base="$2"
    shift 2
  elif [[ "$1" == -* ]]; then
    usage
  else
    paths+=("${1#"$root"/}")
    shift
  fi
done
if [[ -z "$base" ]]; then
  if [[ ${#paths[@]} -gt 0 ]]; then
    base=HEAD
  else
    base="$(git merge-base HEAD origin/staging)"
  fi
fi
base="$(git rev-parse --verify --quiet "$base^{commit}")" || {
  echo "swiftlint-ratchet: base revision does not resolve" >&2
  exit 64
}

work="$(mktemp -d)"
mkdir -p .build
base_root="$(mktemp -d "$root/.build/swiftlint-base.XXXXXX")"
trap 'rm -rf "$work" "$base_root"' EXIT
base_dir="${base_root#"$root"/}"

{
  git diff --name-status -M --diff-filter=AMR "$base" -- "${trees[@]}" |
    awk -F'\t' '$1 ~ /^R/ { print $3 "\t" $2; next } $1 == "M" { print $2 "\t" $2; next } { print $2 "\t" }'
  git ls-files --others --exclude-standard -- "${trees[@]}" | awk '{ print $0 "\t" }'
} | awk -F'\t' '$1 ~ /\.swift$/' | LC_ALL=C sort -u >"$work/changes"
if [[ ${#paths[@]} -gt 0 ]]; then
  printf '%s\n' "${paths[@]}" >"$work/named"
  awk -F'\t' 'FILENAME == ARGV[1] { named[$0] = 1; next } $1 in named' "$work/named" "$work/changes" >"$work/kept"
  mv "$work/kept" "$work/changes"
fi

heads=()
olds=()
while IFS=$'\t' read -r new old; do
  heads+=("$new")
  if [[ -n "$old" ]]; then
    mkdir -p "$base_dir/$(dirname "$new")"
    git show "$base:$old" >"$base_dir/$new"
    olds+=("$base_dir/$new")
  fi
done <"$work/changes"
short="$(git rev-parse --short "$base")"
if [[ ${#heads[@]} -eq 0 ]]; then
  echo "swiftlint-ratchet: no changed Swift files against $short"
  exit 0
fi

native=swiftlint
if command -v "$native" >/dev/null 2>&1 && [[ "$("$native" version 2>/dev/null)" == "$version" ]]; then
  lint=("$native")
  mount="$root/"
else
  if ! "$root/scripts/require-docker.sh" >&2; then
    echo "swiftlint skipped: start Docker (or install swiftlint $version)" >&2
    exit 3
  fi
  lint=(docker run --rm -v "$root:/repo" -w /repo --entrypoint swiftlint "$image")
  mount="/repo/"
fi

read -r -d '' parse_csv <<'AWK' || true
  NR == 1 { next }
  {
    n = split($0, f, ",")
    file = f[1]
    if (index(file, mount) == 1) file = substr(file, length(mount) + 1)
    else if (index(file, root) == 1) file = substr(file, length(root) + 1)
    if (index(file, prefix) == 1) file = substr(file, length(prefix) + 1)
    reason = $0
    sub(/^[^,]*,[^,]*,[^,]*,[^,]*,[^,]*,/, "", reason)
    sub(/,[^,]*$/, "", reason)
    gsub(/^"|"$/, "", reason)
    gsub(/""/, "\"", reason)
    printf "%s\t%s\t%s\t%s\t%s\n", f[n], file, f[2], f[3], reason
  }
AWK

read -r -d '' compare <<'AWK' || true
  FILENAME == ARGV[1] { base[$1 "\t" $2] = $3; next }
  {
    key = $1 "\t" $2
    want = (key in base) ? base[key] : 0
    if ($3 > want) print key "\t" want "\t" $3
  }
AWK

read -r -d '' report <<'AWK' || true
  FILENAME == ARGV[1] { grew[$1 "\t" $2] = 1; order[++n] = $0; next }
  ($1 "\t" $2) in grew { hits[$1 "\t" $2] = hits[$1 "\t" $2] "  " $2 ":" $3 ": " $1 ": " $5 ": " source($2, $3) "\n" }
  END {
    for (i = 1; i <= n; i++) {
      split(order[i], d, "\t")
      print "swiftlint grew: " d[1] " " d[2] " " d[3] " -> " d[4]
      printf "%s", hits[d[1] "\t" d[2]]
    }
  }
  function source(path, line,    text, i) {
    if (!(path in loaded)) {
      loaded[path] = 1
      i = 0
      while ((getline text < path) > 0) src[path, ++i] = text
      close(path)
    }
    text = src[path, line]
    sub(/^[ \t]+/, "", text)
    return text
  }
AWK

run_lint() {
  local name="$1" prefix="$2" rc=0
  shift 2
  "${lint[@]}" lint --quiet --force-exclude --reporter csv "$@" >"$work/$name.csv" 2>"$work/err" || rc=$?
  if [[ $rc -ne 0 && $rc -ne 2 ]]; then
    cat "$work/err" >&2
    echo "swiftlint-ratchet: swiftlint exited $rc" >&2
    exit "$rc"
  fi
  awk -v mount="$mount" -v root="$root/" -v prefix="$prefix" "$parse_csv" "$work/$name.csv" |
    LC_ALL=C sort -u -t $'\t' -k1,1 -k2,2 -k3,3n -k4,4n >"$work/$name.found"
  awk -F'\t' '{ c[$1 "\t" $2]++ } END { for (k in c) print k "\t" c[k] }' "$work/$name.found" | LC_ALL=C sort >"$work/$name.counts"
}

run_lint head "" "${heads[@]}"
: >"$work/base.counts"
if [[ ${#olds[@]} -gt 0 ]]; then
  run_lint base "$base_dir/" "${olds[@]}"
fi

awk -F'\t' "$compare" "$work/base.counts" "$work/head.counts" | LC_ALL=C sort >"$work/grew"
awk -F'\t' "$report" "$work/grew" "$work/head.found"

if [[ -s "$work/grew" ]]; then
  if [[ "${PR_LABELS:-}" == *'"gate-change-approved"'* ]]; then
    echo "swiftlint growth approved by gate-change-approved"
    exit 0
  fi
  exit 1
fi
echo "swiftlint-ratchet: ${#heads[@]} changed files, no count above $short"
