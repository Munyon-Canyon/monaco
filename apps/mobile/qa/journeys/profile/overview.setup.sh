#!/usr/bin/env bash
# journey.py runs this before each scenario. It completes actor A's onboarding (P2) and hands the test a
# cabal where A holds shares (P3), handing the test its id and name, and a resolved call this month so the backend gives A a standing (P4).
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
# shellcheck disable=SC1091
source scripts/qa/seed.sh

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
    if item["share_units"] <= 0:
        continue
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
  # A holds no shares: load the journey catalogue (stocks/browse seeds JRNYAx), then fund a cabal and buy through the product path on the fakes.
  apps/mobile/qa/journeys/stocks/browse.setup.sh "overview-$scenario"
  qa_api_ready
  cabal_id="$(qa_fake_holding A JRNYAx 25 "$name")" || fail "could not seed a funded cabal '$name' as actor A"
fi
read_standing() {
  curl -fsS "$api/v1/users/$user_id" -H "Authorization: Bearer $token" |
    python3 -c 'import json, sys; print("1" if json.load(sys.stdin).get("standing") else "0")'
}

# A product-path call resolves 24 h after its decision, so a run cannot produce one: seed F19aResolveCallsOK and
# move one seeded voter's call_voters to A, as ranking/leaderboards does. Idempotent on a persistent slot database:
# seed only while A has no resolved call this month, so a rerun neither stacks calls nor moves rows already A's.
resolved="$(sql -v me="$user_id" <<<"SELECT count(*) FROM call_voters v JOIN calls c ON c.proposal_id = v.proposal_id
  WHERE v.user_id = :'me' AND c.state = 'resolved' AND c.resolved_at >= date_trunc('month', now())")" ||
  fail "could not count actor A's resolved calls"
if [[ "$resolved" -eq 0 ]]; then
  seed="$(qa_flow_seed 19a ok)" || fail "could not seed F19aResolveCallsOK"
  voter="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["ids"]["user_2"])' <<<"$seed")"
  sql -v me="$user_id" -v voter="$voter" >/dev/null <<<"UPDATE call_voters SET user_id = :'me' WHERE user_id = :'voter'"
  triggered="$(sql <<<"INSERT INTO call_board_triggers (reason, created_at) VALUES ('qa_journey', now()) RETURNING created_at" | head -1)"
  rebuilt=0
  for _ in $(seq 20); do
    rebuilt="$(sql -v t="$triggered" <<<"SELECT count(*) FROM call_board_runs WHERE started_at > :'t'::timestamptz")"
    [[ "$rebuilt" -gt 0 ]] && break
    sleep 1
  done
  [[ "$rebuilt" -gt 0 ]] ||
    fail "the ranking.call_board poller did not rebuild the board within 20 s of the trigger at $triggered: is the worker running?"
fi
standing="$(read_standing)" || fail "could not read actor A's profile"
[[ "$standing" == 1 ]] || fail "the backend returns no standing for actor A after seeding a resolved call"
hand_off cabalID "$cabal_id"
hand_off cabalName "$name"
echo "$scenario: actor A is ONBOARDING_COMPLETED, standing=$standing, and the cabal is '$name' ($cabal_id)"
