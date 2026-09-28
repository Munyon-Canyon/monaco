#!/bin/bash
# Usage: carry.sh <verified-sha> <new-sha>   (run inside any worktree of the repo)
# Exit 0 and print CARRY when both heads have the same tree, or when the change each head adds over its merge-base with origin/backend-rewrite-3
# has the same stable patch-id, so a verdict at <verified-sha> still holds at <new-sha>. Exit 1 (REVERIFY) otherwise.
set -euo pipefail
git fetch -q origin backend-rewrite-3
pid() { git diff "$(git merge-base origin/backend-rewrite-3 "$1")" "$1" | git patch-id --stable | cut -d' ' -f1; }
if [[ "$(git rev-parse "$1^{tree}")" == "$(git rev-parse "$2^{tree}")" ]]; then echo "CARRY $1 -> $2 same tree"; exit 0; fi
a=$(pid "$1"); b=$(pid "$2")
if [[ -n "$a" && "$a" == "$b" ]]; then echo "CARRY $1 -> $2 patch-id $a"; else echo "REVERIFY $1 ($a) -> $2 ($b)"; exit 1; fi
