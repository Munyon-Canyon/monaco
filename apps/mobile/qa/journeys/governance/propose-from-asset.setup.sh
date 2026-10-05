#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
scenario="${1:?journey.py passes the scenario id}"
# shellcheck disable=SC1091
source scripts/qa/seed.sh

if [[ "$scenario" != S1 ]]; then
  echo "no setup for scenario $scenario" >&2
  exit 1
fi

qa_api_ready
did="$(apps/mobile/qa/journeys/privy-user-id.sh A)"
qa_sql -v did="$did" >/dev/null \
  <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now() WHERE privy_user_id = :'did' AND auth_state <> 'ONBOARDING_COMPLETED'"
qa_api A POST /v1/cabals \
  "{\"name\":\"QA asset $run\",\"join_mode\":\"open\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" >/dev/null
echo "seeded: A created the open cabal 'QA asset $run'"
