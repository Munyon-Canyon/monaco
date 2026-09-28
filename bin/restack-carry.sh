#!/bin/bash
# Usage: restack-carry.sh <worktree> <pr>:<verified-patch-id>...   (remaining stack, bottom to top)
# gt sync + restack onto the tip, then check each PR's own diff (parent head..head) still has its verified stable
# patch-id (first 12 chars). Push and re-post verify only when all match; otherwise stop with REVERIFY.
set -euo pipefail
wt=$1; shift
B=/Users/logno/Developer/monaco/.git/pstack/m7-rest/bin
cd "$wt"
first=${1%%:*}
[[ "$(gh pr view "$first" --json baseRefName -q .baseRefName)" == backend-rewrite-3 ]] || gh pr edit "$first" --base backend-rewrite-3 >/dev/null
gt sync --no-interactive >/dev/null 2>&1 || true
gt restack >/dev/null 2>&1
git fetch -q origin
prev=origin/backend-rewrite-3; brs=()
for pair in "$@"; do
  pr=${pair%%:*}; want=${pair#*:}
  br=$(gh pr view "$pr" --json headRefName -q .headRefName); brs+=("$pr:$br")
  got=$(git diff "$prev" "$br" | git patch-id --stable | cut -c1-12)
  [[ "$got" == "$want" ]] || { echo "#$pr patch changed ($want -> $got): REVERIFY, not pushing"; exit 1; }
  prev=$br
done
for x in "${brs[@]}"; do
  br=${x#*:}
  [[ "$(git rev-parse "$br")" == "$(git rev-parse "origin/$br")" ]] || git push -q --force-with-lease="refs/heads/$br:$(git rev-parse "origin/$br")" origin "$br"
done
for x in "${brs[@]}"; do
  pr=${x%%:*}; br=${x#*:}
  sha=$(git rev-parse "$br")
  for i in $(seq 1 20); do [[ "$(gh pr view "$pr" --json headRefOid -q .headRefOid)" == "$sha" ]] && break; sleep 3; done
  $B/post-verify.sh "$pr" "$sha" success "carry: own patch-id unchanged after restack"
done
