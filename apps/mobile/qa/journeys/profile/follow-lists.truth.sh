#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"

a="$(qa_user_id A)" || exit 2
b="$(qa_user_id B)" || exit 2
if ! row="$(qa_sql -F $'\t' -v a="$a" -v b="$b" <<<"SELECT
  EXISTS (SELECT 1 FROM follows WHERE follower_id = :'b' AND followee_id = :'a' AND deleted_at IS NULL),
  EXISTS (SELECT 1 FROM follows WHERE follower_id = :'a' AND followee_id = :'b' AND deleted_at IS NULL)")"; then
  echo "database cannot be reached" >&2
  exit 2
fi
IFS=$'\t' read -r b_follows_a a_follows_b <<<"$row"
if [[ "$b_follows_a" != t ]]; then
  echo "B no longer follows A"
  exit 1
fi
if [[ "$a_follows_b" != t ]]; then
  echo "A no longer follows B"
  exit 1
fi
echo "B still follows A and A still follows B"
