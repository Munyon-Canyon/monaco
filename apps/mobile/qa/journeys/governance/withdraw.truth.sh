#!/usr/bin/env bash
# The ground truth of docs/journeys/governance/withdraw.md: S1's proposal is withdrawn.
# Exits 1 when the row is wrong and 2 when it cannot be read.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
if [[ "${WITHDRAW_TRUTH_ENV:-}" != 1 ]]; then
  WITHDRAW_TRUTH_ENV=1 exec scripts/with-dotenv-local.sh "$0" "$@" 2> >(grep -v -e '^with-dotenv-local:' -e 'injected env' >&2)
fi

: "${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"

unreadable() {
  echo "$*" >&2
  exit 2
}

proposal_id="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get("proposalID", ""))' \
  "$MONACO_QA_HANDOFF" 2>/dev/null || true)"
[[ -n "$proposal_id" ]] || unreadable "the hand-off has no proposalID: S1's setup did not run"

status=0
row="$(apps/mobile/qa/journeys/psql.sh -v ON_ERROR_STOP=1 -tA -v id="$proposal_id" \
  <<<"SELECT status FROM proposals WHERE id = :'id'")" || status=$?
[[ $status -eq 127 ]] && unreadable "psql not found (install it, or start Compose postgres)"
[[ $status -ne 0 ]] && unreadable "database cannot be reached"
[[ -n "$row" ]] || unreadable "proposal $proposal_id has no row"

if [[ "$row" != withdrawn ]]; then
  echo "proposal $proposal_id is '$row', not 'withdrawn'"
  exit 1
fi
echo "proposal $proposal_id is withdrawn"
