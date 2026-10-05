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

create_cabal() {
  qa_api A POST /v1/cabals \
    "{\"name\":\"$1\",\"join_mode\":\"open\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])'
}

ready_actor A
case "$scenario" in
  S1)
    cabal="$(create_cabal "QA paused $run")"
    _qa_monacoctl ops pause --cabal "$cabal" --note "QA $run" >/dev/null
    echo "seeded: A created 'QA paused $run' and ops paused it"
    ;;
  S2)
    create_cabal "QA running $run" >/dev/null
    echo "seeded: A created 'QA running $run'"
    ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac
