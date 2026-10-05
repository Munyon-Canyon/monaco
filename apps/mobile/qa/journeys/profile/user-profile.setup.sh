#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
scenario="${1:?journey.py passes the scenario id}"
: "${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"
qa_api_ready

hand_off() {
  python3 - "$MONACO_QA_HANDOFF" "$1" "$2" <<'PY'
import json, os, sys
path, key, value = sys.argv[1:]
values = json.load(open(path)) if os.path.exists(path) else {}
values[key] = value
json.dump(values, open(path, "w"))
PY
}

ready_actor() {
  local actor="$1" did name
  did="$(_qa_account "$actor" privy_user_id)"
  qa_sql -v did="$did" >/dev/null \
    <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now() WHERE privy_user_id = :'did' AND auth_state <> 'ONBOARDING_COMPLETED'"
  name="$(_qa_account "$actor" name)"
  qa_api "$actor" PATCH /v1/me "{\"display_name\":\"$name\"}" >/dev/null
}

ready_actor A
ready_actor B
me="$(qa_user_id A)"
member="$(qa_user_id B)"
name="QA profile $run"

cabal="$(qa_sql -v name="$name" <<<"SELECT id FROM cabals WHERE name = :'name' LIMIT 1")"
if [[ -z "$cabal" ]]; then
  cabal="$(qa_api A POST /v1/cabals \
    "{\"name\":\"$name\",\"join_mode\":\"open\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" |
    python3 -c 'import json, sys; print(json.load(sys.stdin)["id"])')"
fi
if [[ "$(qa_sql -v c="$cabal" -v u="$member" <<<"SELECT count(*) FROM cabal_members WHERE cabal_id = :'c' AND user_id = :'u'")" == 0 ]]; then
  qa_api B POST "/v1/cabals/$cabal/members" >/dev/null
fi
hand_off cabalID "$cabal"
hand_off cabalName "$name"
hand_off meID "$me"
hand_off memberID "$member"

case "$scenario" in
  S2)
    qa_api A DELETE "/v1/users/$member/follow" >/dev/null
    hand_off followRowsBefore "$(qa_sql -v a="$me" -v b="$member" \
      <<<"SELECT count(*) FROM follows WHERE follower_id = :'a' AND followee_id = :'b'")"
    echo "seeded: A and B share '$name', and A does not follow B"
    ;;
  S1 | S3 | S4 | S5)
    echo "seeded: A and B share '$name'"
    ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac
