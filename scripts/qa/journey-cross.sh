#!/usr/bin/env bash
# Usage: scripts/qa/journey-cross.sh <full-pr-head-sha>
#
# Merge-time journey cross-check. CI runs `journey.py check` on a PR's merge commit against the
# staging it saw. Staging may have moved by the time the PR is squash-merged, so two PRs can each
# pass alone and break journeys together. This merges the head onto the current base in memory,
# runs the check on both, and fails when the merge adds an error the base does not have.
#
# BASE overrides the base (default origin/staging).
# Exit 0: no new errors. Exit 1: the merge adds errors (printed). Exit 2: the head does not merge cleanly.
set -euo pipefail

head=${1:?usage: journey-cross.sh <full-pr-head-sha>}
repo=$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)
cd "$repo"

scratch=$(mktemp -d)
cleanup() {
  local dir
  for dir in "$scratch"/base "$scratch"/merged; do
    git -C "$repo" worktree remove --force "$dir" >/dev/null 2>&1 || true
  done
  rm -rf "$scratch"
  git -C "$repo" worktree prune >/dev/null 2>&1 || true
}
trap cleanup EXIT

git fetch -q origin staging "$head" 2>/dev/null || git fetch -q origin staging 2>/dev/null || true
base=$(git rev-parse "${BASE:-origin/staging}")
if ! tree=$(git merge-tree --write-tree "$base" "$head" 2>/dev/null | head -1); then
  echo "journey-cross: $head does not merge cleanly onto staging"
  exit 2
fi
merged=$(git commit-tree "$tree" -p "$base" -m "journey-cross $head")

errors() {
  (cd "$scratch/$1" && python3 scripts/qa/journey.py check 2>&1 || true) |
    grep -v '^warning:' | grep -E ': ' | grep -v ' journeys checked' | sort -u || true
}

git worktree add -q -f --detach "$scratch/base" "$base"
git worktree add -q -f --detach "$scratch/merged" "$merged"
errors base >"$scratch/base.txt" &
errors merged >"$scratch/merged.txt"
wait
new=$(grep -vxFf "$scratch/base.txt" "$scratch/merged.txt" || true)
if [[ -n "$new" ]]; then
  echo "journey-cross: FAIL, merging $head into staging adds:"
  echo "$new"
  exit 1
fi
echo "journey-cross: ok ($(wc -l <"$scratch/base.txt" | tr -d ' ') errors already on staging, none added)"
