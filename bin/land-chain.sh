#!/bin/bash
# Usage: land-chain.sh <worktree> <pr>:<verified-patch-id>...   (bottom to top)
# Lands a verified stack bottom-up. When the next PR turns DIRTY after its parent's squash, restack-carry.sh restacks
# the rest, checks every patch-id is unchanged, pushes and re-posts verify. Stops on a changed patch or a 45-minute wait.
set -uo pipefail
B=/Users/logno/Developer/monaco/.git/pstack/m7-rest/bin
wt=$1; shift
while (( $# )); do
  pr=${1%%:*}
  [[ "$(gh pr view "$pr" --json baseRefName -q .baseRefName)" == backend-rewrite-3 ]] || gh pr edit "$pr" --base backend-rewrite-3 >/dev/null
  gh pr merge "$pr" --auto --squash >/dev/null 2>&1
  st=
  for i in $(seq 1 90); do
    read -r st ms < <(gh pr view "$pr" --json state,mergeStateStatus -q '.state+" "+.mergeStateStatus' 2>/dev/null)
    [[ "$st" == MERGED ]] && { echo "#$pr MERGED"; break; }
    [[ "$st" == CLOSED ]] && { echo "#$pr CLOSED unmerged, stopping"; exit 1; }
    if [[ "$ms" == DIRTY ]]; then
      $B/restack-carry.sh "$wt" "$@" >/dev/null || { echo "#$pr restack changed a patch, stopping"; exit 1; }
      gh pr merge "$pr" --auto --squash >/dev/null 2>&1
    fi
    sleep 30
  done
  [[ "$st" == MERGED ]] || { echo "#$pr not merged after 45 min, stopping"; exit 1; }
  shift
done
echo "chain landed"
