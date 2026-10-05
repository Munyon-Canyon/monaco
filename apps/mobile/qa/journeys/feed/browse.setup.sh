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
  qa_api "$1" POST /v1/cabals \
    "{\"name\":\"$2\",\"join_mode\":\"open\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])'
}

# The shape of the testkit scenario feed-two-cabals, seeded as the QA accounts.
case "$scenario" in
  S1 | S2 | S3)
    ready_actor A
    ready_actor B
    qa_api A DELETE "/v1/users/$(qa_user_id B)/follow" >/dev/null
    cabal="$(create_cabal B "QA feed $run")"
    qa_api A POST "/v1/cabals/$cabal/members" >/dev/null
    create_cabal A "QA own $run" >/dev/null
    echo "seeded: B created 'QA feed $run' and A joined it, A created 'QA own $run', A follows nobody seeded"
    ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac
