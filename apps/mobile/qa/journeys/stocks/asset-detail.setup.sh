#!/usr/bin/env bash
# S1 needs A to vote in at least two cabals so Propose buy opens the cabal picker (P5). Earlier runs leave cabals, and cabal creation is rate limited, so it creates only the shortfall.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
scenario="${1:?usage: asset-detail.setup.sh <scenario>}"
apps/mobile/qa/journeys/stocks/browse.setup.sh "$scenario"
[[ "$scenario" == S1 ]] || exit 0

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
# shellcheck disable=SC1091
source scripts/qa/seed.sh
qa_api_ready
did="$(apps/mobile/qa/journeys/privy-user-id.sh A)"
qa_sql -v did="$did" >/dev/null \
  <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now() WHERE privy_user_id = :'did' AND auth_state <> 'ONBOARDING_COMPLETED'"
have="$(qa_sql -v did="$did" <<<"SELECT count(*) FROM cabal_members m JOIN users u ON u.id = m.user_id WHERE u.privy_user_id = :'did' AND m.can_vote")"
for ((n = have + 1; n <= 2; n++)); do
  qa_api A POST /v1/cabals \
    "{\"name\":\"QA stocks $run $n\",\"join_mode\":\"open\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" >/dev/null
done
echo "seeded: A votes in $(( have > 2 ? have : 2 )) cabals"
