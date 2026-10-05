#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
scenario="${1:?journey.py passes the scenario id}"
# shellcheck source=/dev/null
source scripts/qa/seed.sh
qa_api_ready

field() {
  python3 -c 'import json,sys; v=json.load(sys.stdin).get(sys.argv[1]); print("" if v is None else v)' "$1"
}

ready_a() {
  local name
  qa_sql -v id="$(qa_user_id A)" >/dev/null \
    <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now() WHERE id = :'id' AND auth_state <> 'ONBOARDING_COMPLETED'"
  name="$(_qa_account A name)"
  if [[ "$(qa_api A GET /v1/me | field display_name)" != "$name" ]]; then
    qa_api A PATCH /v1/me "{\"display_name\":\"$name\"}" >/dev/null
  fi
}

create_cabal() {
  qa_api A POST /v1/cabals \
    "{\"name\":\"$1\",\"join_mode\":\"open\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" |
    field id
}

ready_a
case "$scenario" in
  S1 | S2)
    name="QA fund $run"
    if [[ "$(qa_sql -v name="$name" <<<"SELECT count(*) FROM cabals WHERE name = :'name'")" == 0 ]]; then
      create_cabal "$name" >/dev/null
    fi
    echo "seeded: A created the open cabal '$name'"
    ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac
