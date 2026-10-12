#!/usr/bin/env bash
# Measure what one stage 0 check costs the machine.
#
#   scripts/qa/measure-check.sh <label> <head-sha> <base-sha> [out.tsv] [onto]
#
# Checks out <head-sha> on a throwaway branch in its own worktree (or, with [onto], replays
# <head-sha>'s own change on top of <onto> so the check runs with <onto>'s tooling), runs
# `monacoctl agents check --fresh --base <base-sha>` there, and appends one TSV row:
# label, head, wall seconds, check CPU-seconds (user+sys of the check and every
# process it waited for), test-database CPU-seconds (cgroup usage of the
# monaco-postgres-test* containers), host disk bytes written (all internal disks,
# from IOBlockStorageDriver's counters), Go build cache growth in bytes, and the
# check's exit code. Run one at a time on a quiet machine: the disk and database
# columns are machine-wide.
set -euo pipefail

label="$1"
head="$2"
base="$3"
out="${4:-$(git rev-parse --path-format=absolute --git-common-dir)/pstack/measure-check.tsv}"
onto="${5:-}"

primary="$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")"
monacoctl="$primary/bin/monacoctl"
tree="$primary/.worktrees/measure-$label"
branch="measure-$label"

db_cpu_usec() {
  local total=0 c v
  for c in $(docker ps --format '{{.Names}}' | grep '^monaco-postgres-test'); do
    v="$(docker exec "$c" cat /sys/fs/cgroup/cpu.stat 2>/dev/null | awk '$1 == "usage_usec" { print $2 }')"
    total=$((total + ${v:-0}))
  done
  echo "$total"
}

disk_written() {
  ioreg -c IOBlockStorageDriver -r -w0 | grep -oE '"Bytes \(Write\)"=[0-9]+' | cut -d= -f2 |
    awk '{ s += $1 } END { printf "%d\n", s }'
}

gocache_bytes() {
  du -sk "$(go env GOCACHE)" | awk '{ print $1 * 1024 }'
}

if [[ ! -e "$out" ]]; then
  printf 'label\thead\twall_s\tcheck_cpu_s\tdb_cpu_s\tdisk_written_b\tgocache_growth_b\texit\n' > "$out"
fi

git -C "$primary" worktree remove --force "$tree" >/dev/null 2>&1 || true
git -C "$primary" branch -D "$branch" >/dev/null 2>&1 || true
if [[ -n "$onto" ]]; then
  git -C "$primary" worktree add -q -b "$branch" "$tree" "$onto"
  git -C "$tree" -c user.name=measure -c user.email=measure@example.com cherry-pick --allow-empty "$head" >/dev/null
  base="$onto"
else
  git -C "$primary" worktree add -q -b "$branch" "$tree" "$head"
fi
ln -s "$primary/.bin" "$tree/.bin"

db0="$(db_cpu_usec)"
disk0="$(disk_written)"
cache0="$(gocache_bytes)"
start="$(date +%s)"
timings="$(mktemp)"
rc=0
(cd "$tree" && /usr/bin/time -l "$monacoctl" agents check --fresh --base "$base") > "$timings.log" 2> "$timings" || rc=$?
wall=$(($(date +%s) - start))
db1="$(db_cpu_usec)"
disk1="$(disk_written)"
cache1="$(gocache_bytes)"

cpu="$(awk '/ user / { u = $3 } / sys / { s = $5 } /real.*user.*sys/ { u = $3; s = $5 } END { printf "%.1f\n", u + s }' "$timings")"
printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$label" "${head:0:12}" "$wall" "$cpu" \
  "$(awk -v a="$db0" -v b="$db1" 'BEGIN { printf "%.1f\n", (b - a) / 1e6 }')" \
  "$((disk1 - disk0))" "$((cache1 - cache0))" "$rc" >> "$out"
tail -n 1 "$out"
tail -n 3 "$timings.log" >&2

git -C "$primary" worktree remove --force "$tree" >/dev/null 2>&1 || true
git -C "$primary" branch -D "$branch" >/dev/null 2>&1 || true
rm -f "$timings" "$timings.log"
