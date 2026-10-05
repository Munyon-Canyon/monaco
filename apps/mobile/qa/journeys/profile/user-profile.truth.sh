#!/usr/bin/env bash
set -euo pipefail

: "${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"

read_handoff() {
  python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get(sys.argv[2], ""))' "$MONACO_QA_HANDOFF" "$1"
}

before="$(read_handoff followRowsBefore)"
if [[ -z "$before" ]]; then
  echo "S2 did not run: no follow to check"
  exit 0
fi
me="$(read_handoff meID)"
member="$(read_handoff memberID)"
if ! row="$(qa_sql -F $'\t' -v a="$me" -v b="$member" <<<"SELECT count(*), count(*) FILTER (WHERE deleted_at IS NULL)
  FROM follows WHERE follower_id = :'a' AND followee_id = :'b'")"; then
  echo "database cannot be reached" >&2
  exit 2
fi
IFS=$'\t' read -r rows live <<<"$row"
if ((rows <= before)); then
  echo "no new A-to-B row in follows: S2's Follow never reached the backend"
  exit 1
fi
if ((live != 0)); then
  echo "A still follows B: S2's unfollow never reached the backend"
  exit 1
fi
echo "A followed B and unfollowed B ($((rows - before)) new row, none live)"
