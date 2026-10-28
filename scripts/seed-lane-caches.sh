#!/usr/bin/env bash
# Seeds a new lane's Swift and Xcode build caches from the primary checkout: an APFS clone (cp -c) of
# packages/mobile-core/.build and .build/DerivedData, so the lane's first swift test and xcodebuild are not cold.
# Usage: seed-lane-caches.sh <worktree>. Prints "seeded <dir> from <src>" or "skipped <dir>: <reason>" per
# directory and always exits 0, so a dispatch never fails on it. Each lane keeps its own copy: two xcodebuild
# runs on one DerivedData fail with "database is locked".
set -uo pipefail

worktree="${1:-}"
dirs=(packages/mobile-core/.build .build/DerivedData)

skip_all() {
  local d
  for d in "${dirs[@]}"; do echo "skipped $d: $1"; done
  exit 0
}

[[ -n "$worktree" && -d "$worktree" ]] || skip_all "no worktree"
[[ "$(uname -s)" == Darwin ]] || skip_all "cp -c needs APFS on macOS"
common="$(git -C "$worktree" rev-parse --path-format=absolute --git-common-dir 2>/dev/null)" || skip_all "not a git worktree"
primary="$(dirname "$common")"
[[ "$(cd "$primary" && pwd -P)" != "$(cd "$worktree" && pwd -P)" ]] || skip_all "worktree is the primary checkout"

for d in "${dirs[@]}"; do
  src="$primary/$d"
  dst="$worktree/$d"
  if [[ ! -d "$src" ]]; then
    echo "skipped $d: no source at $src"
  elif [[ -e "$dst" ]]; then
    echo "skipped $d: destination exists"
  elif mkdir -p "$(dirname "$dst")" && out="$(cp -c -R "$src" "$dst" 2>&1)"; then
    echo "seeded $d from $src"
  else
    rm -rf "$dst"
    echo "skipped $d: clone failed: ${out:-mkdir failed}"
  fi
done
exit 0
