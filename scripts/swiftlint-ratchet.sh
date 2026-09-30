#!/usr/bin/env bash
set -euo pipefail

version=0.65.0
image="ghcr.io/realm/swiftlint:$version"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$root"
baseline=.swiftlint-baseline.tsv

update=0
paths=()
for arg in "$@"; do
  case "$arg" in
    --update) update=1 ;;
    -*)
      echo "usage: scripts/swiftlint-ratchet.sh [--update] [path...]" >&2
      exit 64
      ;;
    *) paths+=("${arg#"$root"/}") ;;
  esac
done
full=0
if [[ ${#paths[@]} -eq 0 ]]; then
  paths=(apps/mobile packages/mobile-core)
  full=1
fi
if [[ $update -eq 1 && $full -eq 0 ]]; then
  echo "swiftlint-ratchet: --update runs on the whole tree, so pass no paths" >&2
  exit 64
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
    got[key] = $3
    want = (key in base) ? base[key] : 0
    if ($3 > want) print "grew\t" key "\t" want "\t" $3
  }
  END {
    if (!full) exit
    for (key in base) {
      have = (key in got) ? got[key] : 0
      if (have < base[key]) print "lower\t" key "\t" base[key] "\t" have
    }
  }
AWK

read -r -d '' report <<'AWK' || true
  FILENAME == ARGV[1] {
    if ($1 == "grew") { grew[$2 "\t" $3] = 1; order[++n] = $0 }
    else if (!update) print "lower the baseline: " $2 " " $3 " " $4 " -> " $5
    next
  }
  ($1 "\t" $2) in grew { hits[$1 "\t" $2] = hits[$1 "\t" $2] "  " $2 ":" $3 ": " $1 ": " $5 ": " source($2, $3) "\n" }
  END {
    for (i = 1; i <= n; i++) {
      split(order[i], d, "\t")
      print "swiftlint grew: " d[2] " " d[3] " " d[4] " -> " d[5]
      printf "%s", hits[d[2] "\t" d[3]]
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

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

rc=0
"${lint[@]}" lint --quiet --force-exclude --reporter csv "${paths[@]}" >"$work/csv" 2>"$work/err" || rc=$?
if [[ $rc -ne 0 && $rc -ne 2 ]]; then
  cat "$work/err" >&2
  echo "swiftlint-ratchet: swiftlint exited $rc" >&2
  exit "$rc"
fi

awk -v mount="$mount" -v root="$root/" "$parse_csv" "$work/csv" | LC_ALL=C sort -u -t $'\t' -k1,1 -k2,2 -k3,3n -k4,4n >"$work/found"

awk -F'\t' '{ c[$1 "\t" $2]++ } END { for (k in c) print k "\t" c[k] }' "$work/found" | LC_ALL=C sort >"$work/counts"
if [[ -f "$baseline" ]]; then
  cp "$baseline" "$work/baseline"
elif [[ $update -eq 1 ]]; then
  cp "$work/counts" "$work/baseline"
else
  : >"$work/baseline"
fi

awk -F'\t' -v full="$full" "$compare" "$work/baseline" "$work/counts" | LC_ALL=C sort >"$work/diff"

awk -F'\t' -v update="$update" "$report" "$work/diff" "$work/found"
grew=0
lower=0
grep -q '^grew' "$work/diff" && grew=1
grep -q '^lower' "$work/diff" && lower=1

if [[ $grew -eq 1 ]]; then
  [[ $update -eq 0 ]] || echo "swiftlint-ratchet: --update never raises a row, so fix the code instead" >&2
  exit 1
fi
if [[ $update -eq 1 ]]; then
  cp "$work/counts" "$baseline"
  echo "swiftlint-ratchet: wrote $baseline ($(wc -l <"$baseline" | tr -d ' ') rows)"
  exit 0
fi
if [[ $lower -eq 1 ]]; then
  echo "swiftlint-ratchet: run scripts/swiftlint-ratchet.sh --update and commit $baseline"
  exit 1
fi
echo "swiftlint-ratchet: $(wc -l <"$work/found" | tr -d ' ') findings, none above $baseline"
