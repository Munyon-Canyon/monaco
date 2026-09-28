#!/usr/bin/env bash
set -euo pipefail

[[ $# -eq 2 ]] || { echo "usage: scripts/ci/checkpoint-tree.sh <squash-sha> <head-sha>" >&2; exit 2; }
REPO="${GITHUB_REPOSITORY:-lognorman20/monaco}"

tree_sha() { gh api "repos/$REPO/git/commits/$1" --jq .tree.sha; }

squash="$(tree_sha "$1")"
head="$(tree_sha "$2")"
if [[ "$squash" != "$head" ]]; then
  echo "main $1 has tree $squash, but the feature branch head $2 has tree $head" >&2
  exit 1
fi
echo "main $1 and the feature branch head $2 share tree $squash"
