#!/usr/bin/env bash
# Trims the Go build cache in two passes, never touching an entry used in the last
# MONACO_GOCACHE_FLOOR_MIN minutes (default 180). A running build looks an action up,
# then opens its output later, so deleting a fresh entry fails that build with
# "go-build/...: no such file or directory" (lane 6942, 2026-10-29 08:10).
#
#   1. Delete entries untouched for MONACO_GOCACHE_AGE_MIN minutes (default 1440).
#   2. If the cache is still above MONACO_GOCACHE_CAP_GB (default 100), delete the
#      oldest remaining entries older than the floor until it fits; when the floor
#      stops it first, say so and stop.
#
# Go refreshes the mtime of an entry when it reuses one more than an hour stale, so mtime
# is the access signal. Prints one line: before, after, files deleted, and why it stopped.
set -euo pipefail

floor_min=${MONACO_GOCACHE_FLOOR_MIN:-180}
age_min=${MONACO_GOCACHE_AGE_MIN:-1440}
cap_gb=${MONACO_GOCACHE_CAP_GB:-100}
cache=${GOCACHE:-$(go env GOCACHE)}
[[ -d "$cache" ]] || { echo "gocache: no cache at $cache"; exit 0; }
((floor_min >= 180)) || { echo "gocache: a floor under 180 minutes deletes outputs running builds still need" >&2; exit 2; }
((age_min >= floor_min)) || age_min=$floor_min

size_kb() { du -sk "$cache" 2>/dev/null | cut -f1; }
gb() { echo $(($1 / 1024 / 1024)); }

before=$(size_kb)
aged=$(find "$cache" -mindepth 2 -type f -mmin +"$age_min" -print -delete | wc -l | tr -d ' ')
after=$(size_kb)
cap_kb=${MONACO_GOCACHE_CAP_KB:-$((cap_gb * 1024 * 1024))}
oldest=0
fits=yes
if ((after > cap_kb)); then
  # Oldest first by mtime, and only entries older than the floor.
  list=$(mktemp "${TMPDIR:-/tmp}/gocache-trim.XXXXXX")
  trap 'rm -f "$list"' EXIT
  if stat -f '%m' / >/dev/null 2>&1; then
    find "$cache" -mindepth 2 -type f -mmin +"$floor_min" -exec stat -f '%m %z %N' {} + 2>/dev/null
  else
    find "$cache" -mindepth 2 -type f -mmin +"$floor_min" -printf '%T@ %s %p\n' 2>/dev/null
  fi | sort -n > "$list"
  over=$(((after - cap_kb) * 1024))
  while read -r _ bytes path && ((over > 0)); do
    rm -f "$path" && oldest=$((oldest + 1)) && over=$((over - bytes))
  done < "$list"
  after=$(size_kb)
  ((after <= cap_kb)) || fits=no
fi
printf 'gocache: %s GB -> %s GB, deleted %s aged and %s oldest entries\n' "$(gb "$before")" "$(gb "$after")" "$aged" "$oldest"
[[ $fits == yes ]] || printf 'gocache: still above the cap, every remaining entry was used in the last %s minutes\n' "$floor_min"
