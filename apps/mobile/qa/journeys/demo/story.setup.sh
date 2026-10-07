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

story_cabal() {
  local name="QA story $run" id
  id="$(qa_sql -v name="$name" <<<"SELECT id FROM cabals WHERE name = :'name'")"
  if [[ -z "$id" ]]; then
    id="$(qa_api A POST /v1/cabals \
      "{\"name\":\"$name\",\"join_mode\":\"request\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" |
      python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
  fi
  qa_admit B A "$id"
  echo "seeded: A created the cabal '$name' and B joined it"
}

ready_actor A
ready_actor B
case "$scenario" in
  S1)
    echo "seeded: A and B are past onboarding"
    ;;
  S3)
    story_cabal
    "$QA_ROOT/apps/mobile/qa/journeys/stocks/browse.setup.sh" S1
    ;;
  S2 | S4 | S5 | S6 | S7 | S8)
    story_cabal
    ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac
