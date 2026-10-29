#!/usr/bin/env bash
# The ground truth of docs/journeys/home/activity.md: A cleared the Results item and nothing else.
# Exits 1 when a row is wrong and 2 when it cannot be read.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
# shellcheck source=/dev/null
source scripts/qa/seed.sh

: "${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"

handoff() {
  python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get(sys.argv[2], ""))' "$MONACO_QA_HANDOFF" "$1" \
    2>/dev/null || true
}

result_key="$(handoff resultKey)"
waiting_key="$(handoff waitingKey)"
reply_key="$(handoff replyKey)"
[[ -n "$result_key" && -n "$waiting_key" && -n "$reply_key" ]] || {
  echo "the hand-off has no resultKey, waitingKey and replyKey: S1's setup did not run" >&2
  exit 2
}

did="$(apps/mobile/qa/journeys/privy-user-id.sh A)"
cleared() {
  qa_sql -v did="$did" -v key="$1" <<<"SELECT EXISTS (SELECT 1 FROM catch_up_state s JOIN users u ON u.id = s.user_id
    WHERE u.privy_user_id = :'did' AND s.item_key = :'key' AND s.cleared_at IS NOT NULL)::text" ||
    { echo "database cannot be reached" >&2; exit 2; }
}

failed=0
if [[ "$(cleared "$result_key")" != true ]]; then
  echo "A's Results item $result_key is not cleared"
  failed=1
fi
for key in "$waiting_key" "$reply_key"; do
  if [[ "$(cleared "$key")" != false ]]; then
    echo "A's item $key was cleared, but the journey only clears the Results item"
    failed=1
  fi
done
[[ $failed -eq 0 ]] || exit 1
echo "ok: A cleared $result_key and nothing else"
