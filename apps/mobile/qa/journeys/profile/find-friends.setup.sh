#!/usr/bin/env bash
set -euo pipefail

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

case "$scenario" in
  S1 | S2 | S3)
    ready_actor A
    ready_actor B
    member="$(qa_user_id B)"
    handle="$(qa_sql -v b="$member" <<<"SELECT coalesce(handle, '') FROM users WHERE id = :'b'")"
    [[ -n "$handle" ]] || { echo "actor B has no handle: finish B's onboarding first" >&2; exit 1; }
    qa_api A DELETE "/v1/users/$member/follow" >/dev/null
    hand_off memberID "$member"
    hand_off memberHandle "$handle"
    hand_off seededAt "$(qa_sql <<<"SELECT now()")"
    echo "seeded: B is @$handle, and A does not follow B"
    ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac
