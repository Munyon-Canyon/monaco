#!/usr/bin/env bash
# journey.py runs this before each scenario. It completes actor A's onboarding (P2) and hands the test a
# cabal A holds (P3), handing the test its id and name, and whether the backend gives A a month standing (P4).
set -euo pipefail

scenario="${1:?usage: overview.setup.sh <scenario>}"
fail() {
  echo "$scenario: $*" >&2
  exit 1
}

if [[ "${OVERVIEW_SETUP_ENV:-}" != 1 ]]; then
  OVERVIEW_SETUP_ENV=1 exec scripts/with-dotenv-local.sh "$0" "$@" 2> >(grep -v -e '^with-dotenv-local:' -e 'injected env' >&2)
fi

: "${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"
run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
api="${MONACO_API_BASE_URL:-http://127.0.0.1:8080}"
name="QA overview $run"

sql() {
  apps/mobile/qa/journeys/psql.sh -v ON_ERROR_STOP=1 -tA "$@"
}

hand_off() {
  python3 - "$MONACO_QA_HANDOFF" "$1" "$2" <<'PY'
import json, os, sys
path, key, value = sys.argv[1:]
values = json.load(open(path)) if os.path.exists(path) else {}
values[key] = value
json.dump(values, open(path, "w"))
PY
}

did="$(apps/mobile/qa/journeys/privy-user-id.sh A)"
user_id="$(sql -v did="$did" <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now()
  WHERE privy_user_id = :'did' RETURNING id" | head -1)"
[[ -n "$user_id" ]] || fail "actor A has no users row. Run auth/sign-in first"

log="$(mktemp)"
token="$(scripts/qa/monacoctl.sh dev token --user "$user_id" --ttl 1h 2>"$log")" ||
  fail "monacoctl dev token for actor A failed: $(grep -v 'injected env' "$log")"

held="$(curl -fsS "$api/v1/me/portfolio" -H "Authorization: Bearer $token" |
  API="$api" TOKEN="$token" python3 -c 'import json, os, sys, urllib.request
cabals = json.load(sys.stdin)["cabals"]
cabals.sort(key=lambda item: (item["share_units"] > 0, item["value_micros"]), reverse=True)
for item in cabals:
    request = urllib.request.Request(
        os.environ["API"] + "/v1/cabals/" + item["cabal"]["id"],
        headers={"Authorization": "Bearer " + os.environ["TOKEN"]})
    with urllib.request.urlopen(request) as response:
        if json.load(response).get("me"):
            print(item["cabal"]["id"] + "|" + item["cabal"]["name"])
            break')" || fail "could not read actor A's portfolio"
if [[ -n "$held" ]]; then
  cabal_id="${held%%|*}"
  name="${held#*|}"
else
  cabal_id="$(sql -v uid="$user_id" -v name="$name" <<<"SELECT c.id FROM cabals c
    JOIN cabal_members m ON m.cabal_id = c.id WHERE m.user_id = :'uid' AND c.name = :'name' LIMIT 1")"
  if [[ -z "$cabal_id" ]]; then
    cabal_id="$(curl -fsS -X POST "$api/v1/cabals" -H "Authorization: Bearer $token" \
      -H 'Content-Type: application/json' -H "Idempotency-Key: $(uuidgen | tr '[:upper:]' '[:lower:]')" \
      -d "{\"name\":\"$name\",\"join_mode\":\"request\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" |
      python3 -c 'import json, sys; print(json.load(sys.stdin)["id"])')" || fail "could not create '$name' as actor A"
  fi
fi
standing="$(curl -fsS "$api/v1/users/$user_id" -H "Authorization: Bearer $token" |
  python3 -c 'import json, sys; print("1" if json.load(sys.stdin).get("standing") else "0")')" ||
  fail "could not read actor A's profile"
hand_off hasStanding "$standing"
hand_off cabalID "$cabal_id"
hand_off cabalName "$name"
echo "$scenario: actor A is ONBOARDING_COMPLETED, standing=$standing, and the cabal is '$name' ($cabal_id)"
