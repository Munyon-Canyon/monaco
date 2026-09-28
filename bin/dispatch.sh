#!/bin/bash
# Usage: dispatch.sh <worktree-name> [<dep-pr>...]
# Land-then-write: refuses unless every dependency PR is MERGED and its merge commit is in origin/backend-rewrite-3.
# Then creates .worktrees/<worktree-name> detached at the current tip and prints the tip SHA.
set -euo pipefail
R=/Users/logno/Developer/monaco; B=backend-rewrite-3
name=$1; shift
git -C "$R" fetch -q origin "$B"
tip=$(git -C "$R" rev-parse "origin/$B")
for pr in "$@"; do
  read -r state mc < <(gh pr view "$pr" --json state,mergeCommit -q '.state+" "+(.mergeCommit.oid // "")')
  if [[ "$state" != MERGED ]] || ! git -C "$R" merge-base --is-ancestor "$mc" "$tip"; then
    echo "dispatch.sh: REFUSED $name: dependency #$pr is $state, not landed in $B@${tip:0:8}" >&2; exit 1
  fi
done
[[ -e "$R/.worktrees/$name" ]] && { echo "dispatch.sh: REFUSED: .worktrees/$name exists" >&2; exit 1; }
git -C "$R/.worktrees/m7-rest-root" worktree add -q --detach "$R/.worktrees/$name" "$tip"
echo "$tip"
