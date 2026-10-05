#!/usr/bin/env bash
set -euo pipefail

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
    echo "seeded: A is onboarded; the runner funds A's wallet with 1 USDC"
    ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac
