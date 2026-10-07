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

hand_off() {
  python3 - "$handoff" "$@" <<'PY'
import json, pathlib, sys
path = pathlib.Path(sys.argv[1])
values = json.loads(path.read_text()) if path.exists() and path.read_text().strip() else {}
pairs = sys.argv[2:]
values.update(zip(pairs[::2], pairs[1::2]))
path.write_text(json.dumps(values))
PY
}

case "$scenario" in
  S1)
    ready_actor A
    name="QA home $run"
    cabal="$(qa_api A POST /v1/cabals \
      "{\"name\":\"$name\",\"join_mode\":\"request\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" |
      python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
    hand_off cabalID "$cabal" cabalName "$name"
    echo "seeded: A created the cabal '$name'"
    ;;
  S2)
    ready_actor C
    for cabal in $(qa_api C GET /v1/me/cabals | python3 -c 'import json,sys; print(" ".join(c["id"] for c in json.load(sys.stdin)))'); do
      qa_api C DELETE "/v1/cabals/$cabal/members/me" >/dev/null
    done
    left="$(qa_api C GET /v1/me/cabals | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')"
    if [[ "$left" != 0 ]]; then
      echo "C still belongs to $left cabal(s) after leaving each one" >&2
      exit 1
    fi
    echo "seeded: C belongs to no cabal"
    ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac
