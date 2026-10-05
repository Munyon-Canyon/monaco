#!/usr/bin/env bash
set -euo pipefail

: "${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"

read_handoff() {
  python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get(sys.argv[2], ""))' "$MONACO_QA_HANDOFF" "$1"
}

member="$(read_handoff memberID)"
seeded="$(read_handoff seededAt)"
[[ -n "$member" && -n "$seeded" ]] || { echo "the setup script handed off no memberID or seededAt" >&2; exit 2; }
a="$(qa_user_id A)" || exit 2
if ! row="$(qa_sql -F $'\t' -v a="$a" -v b="$member" -v t="$seeded" <<<"SELECT
  count(*) FILTER (WHERE deleted_at IS NULL), count(*) FILTER (WHERE deleted_at IS NULL AND created_at <= :'t'::timestamptz)
  FROM follows WHERE follower_id = :'a' AND followee_id = :'b'")"; then
  echo "database cannot be reached" >&2
  exit 2
fi
IFS=$'\t' read -r live stale <<<"$row"
if ((stale != 0)); then
  echo "A's live follow of B predates the setup: the unfollow in setup did not land"
  exit 1
fi
if ((live == 0)); then
  echo "A does not follow B: S2 is blocked on #2142"
  exit 0
fi
echo "A follows B, written by the run's S2"
