#!/usr/bin/env bash
# Clearing the result row marks it cleared in A's catch_up_state and leaves the other items of this run alone.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
# shellcheck source=/dev/null
source scripts/qa/seed.sh

handoff="${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"
result="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["resultProposalID"])' "$handoff")"
waiting="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["proposalID"])' "$handoff")"
reply="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["replyID"])' "$handoff")"
did="$(apps/mobile/qa/journeys/privy-user-id.sh A)"
rows="$(qa_sql -v did="$did" -v result="proposal:$result" -v waiting="proposal:$waiting" -v reply="comment:$reply" <<<"
SELECT coalesce(string_agg(item_key || '=' || (cleared_at IS NOT NULL)::text, ' ' ORDER BY item_key), '')
FROM catch_up_state s JOIN users u ON u.id = s.user_id
WHERE u.privy_user_id = :'did' AND s.cleared_at IS NOT NULL
  AND s.item_key IN (:'result', :'waiting', :'reply')")" ||
  { echo "database cannot be reached" >&2; exit 2; }
if [[ "$rows" != "proposal:$result=true" ]]; then
  echo "A's cleared catch_up_state rows read '$rows', not 'proposal:$result=true'"
  exit 1
fi
echo "ok: only the result row proposal:$result is cleared"
