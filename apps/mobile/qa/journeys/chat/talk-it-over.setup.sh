#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
scenario="${1:?journey.py passes the scenario id}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"
qa_api_ready

ready_actor() {
  local actor="$1" did name
  did="$(_qa_account "$actor" privy_user_id)"
  qa_sql -v did="$did" >/dev/null \
    <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now() WHERE privy_user_id = :'did' AND auth_state <> 'ONBOARDING_COMPLETED'"
  name="$(_qa_account "$actor" name)"
  qa_api "$actor" PATCH /v1/me "{\"display_name\":\"$name\"}" >/dev/null
}

case "$scenario" in
  S1)
    ready_actor A
    ready_actor B
    qa_sql -v did="$(_qa_account A privy_user_id)" >/dev/null <<<"DELETE FROM rate_limit_buckets WHERE key = 'op:postCabal:actor:user:' || (SELECT id FROM users WHERE privy_user_id = :'did')"
    created="$(qa_api A POST /v1/cabals \
      "{\"name\":\"QA $run\",\"join_mode\":\"request\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}")"
    cabal="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])' <<<"$created")"
    qa_admit B A "$cabal"
    echo "seeded: A created 'QA $run' and B joined it"
    ;;
  S2 | S3 | S4 | S5)
    echo "no seeding for $scenario: it continues from the chat S1 created"
    ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac
