#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
scenario="${1:?journey.py passes the scenario id}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"
qa_api_ready

did="$(_qa_account A privy_user_id)"
qa_sql -v did="$did" >/dev/null \
  <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now() WHERE privy_user_id = :'did' AND auth_state <> 'ONBOARDING_COMPLETED'"
qa_api A PATCH /v1/me "{\"display_name\":\"$(_qa_account A name)\"}" >/dev/null

case "$scenario" in
  S1 | S2)
    qa_api A POST /v1/cabals \
      "{\"name\":\"QA bot $run\",\"join_mode\":\"open\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" >/dev/null
    echo "seeded: A created 'QA bot $run' (no bot: nothing seeds one until #691)"
    ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac
