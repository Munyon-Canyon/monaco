#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"
handoff="${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"

handed_off() {
  python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get(sys.argv[2], ""))' "$handoff" "$1"
}

a="$(qa_user_id A)" || exit 2
b="$(qa_user_id B)" || exit 2
after_follow="$(handed_off liveAfterFollow)"
rows_before="$(handed_off followRowsBefore)"
if [[ -z "$after_follow" || -z "$rows_before" ]]; then
  echo "the hand-off has no liveAfterFollow or followRowsBefore: the setup script did not reach S1 and S4"
  exit 1
fi
if ! row="$(qa_sql -F $'\t' -v a="$a" -v b="$b" <<<"SELECT
  count(*) FILTER (WHERE deleted_at IS NULL), count(*)
  FROM follows WHERE follower_id = :'a' AND followee_id = :'b'")"; then
  echo "database cannot be reached" >&2
  exit 2
fi
IFS=$'\t' read -r live total <<<"$row"
if [[ "$after_follow" != 1 ]]; then
  echo "after S3, follows had $after_follow live rows from A to B, want 1"
  exit 1
fi
if [[ "$live" != 0 ]]; then
  echo "after S5, follows has $live live rows from A to B, want 0"
  exit 1
fi
if ((total <= rows_before)); then
  echo "follows has $total rows from A to B, no more than the $rows_before before the run: the follow never reached the backend"
  exit 1
fi
echo "follows had one live row from A to B after S3 and none after S5"
