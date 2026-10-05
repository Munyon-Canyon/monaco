#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
scenario="${1:?journey.py passes the scenario id}"
handoff="${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"
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
  S1 | S2 | S3) ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac

ready_actor A
ready_actor B
ready_actor C
name="QA ranks $run"
cabal="$(qa_sql -v name="$name" <<<"SELECT id FROM cabals WHERE name = :'name' LIMIT 1")"
if [[ -z "$cabal" ]]; then
  cabal="$(qa_api A POST /v1/cabals \
    "{\"name\":\"$name\",\"join_mode\":\"open\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
  qa_api B POST "/v1/cabals/$cabal/members" >/dev/null
  qa_api C POST "/v1/cabals/$cabal/members" >/dev/null
fi

python3 - "$handoff" "$cabal" "$name" "$(qa_user_id B)" "$(_qa_account B name)" <<'PY'
import json, pathlib, sys
path = pathlib.Path(sys.argv[1])
values = json.loads(path.read_text()) if path.exists() and path.read_text().strip() else {}
values.update(cabalID=sys.argv[2], cabalName=sys.argv[3], memberID=sys.argv[4], memberName=sys.argv[5])
path.write_text(json.dumps(values))
PY
echo "seeded: A created the open cabal '$name', B and C joined it"
