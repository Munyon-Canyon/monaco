#!/usr/bin/env bash
# journey.py runs this before each scenario. It completes actor B's onboarding, and before S2 it clears
# B's pending invites and has a dev host invite B to a new cabal (P3), handing the test its name.
set -euo pipefail

scenario="${1:?usage: notifications.setup.sh <scenario>}"
fail() {
  echo "$scenario: $*" >&2
  exit 1
}

if [[ "${NOTIFICATIONS_SETUP_ENV:-}" != 1 ]]; then
  NOTIFICATIONS_SETUP_ENV=1 exec scripts/with-dotenv-local.sh "$0" "$@" 2> >(grep -v -e '^with-dotenv-local:' -e 'injected env' >&2)
fi

: "${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"
run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
api="${MONACO_API_BASE_URL:-http://127.0.0.1:8080}"

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

call() {
  local method="$1" path="$2" bearer="$3" body="${4:-}" args
  args=(-fsS -X "$method" "$api$path" -H "Authorization: Bearer $bearer" -H 'Content-Type: application/json')
  [[ "$method" == GET ]] || args+=(-H "Idempotency-Key: $(uuidgen | tr '[:upper:]' '[:lower:]')")
  [[ -z "$body" ]] || args+=(-d "$body")
  curl "${args[@]}"
}

dev_token() {
  local unset_qa=()
  while read -r name; do unset_qa+=(-u "$name"); done < <(compgen -e | grep '^MONACO_QA_')
  env "${unset_qa[@]}" bin/monacoctl dev token --user "$1" --ttl 1h 2>/dev/null || fail "monacoctl dev token --user $1 failed"
}

[[ -x bin/monacoctl ]] || fail "bin/monacoctl is missing: run just build backend"
did="$(apps/mobile/qa/journeys/privy-user-id.sh B)"
row="$(sql -F ' ' -v did="$did" <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now()
  WHERE privy_user_id = :'did' RETURNING id, handle" | head -1)"
read -r user_id handle <<<"$row"
[[ -n "$user_id" && -n "$handle" ]] || fail "actor B has no users row or no handle. Run auth/sign-in as B first"
echo "$scenario: actor B (@$handle) is ONBOARDING_COMPLETED"
[[ "$scenario" == S2 ]] || exit 0

token_b="$(dev_token "$user_id")"
call GET /v1/me/cabal-invites "$token_b" | python3 -c '
import json, sys
for invite in json.load(sys.stdin):
    print(invite["cabal"]["id"], invite["request_id"])
' | while read -r cabal request; do
  call POST "/v1/cabals/$cabal/access-requests/$request/decision" "$token_b" '{"decision":"deny"}' >/dev/null
done

host="$(dev_token new)"
call PATCH /v1/me "$host" '{"display_name":"QA push host"}' >/dev/null || fail "could not name the dev host"
name="QA push $run"
cabal="$(call POST /v1/cabals "$host" \
  "{\"name\":\"$name\",\"join_mode\":\"request\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" |
  python3 -c 'import json, sys; print(json.load(sys.stdin)["id"])')" || fail "could not create '$name'"
call POST "/v1/cabals/$cabal/invites" "$host" "{\"handle\":\"$handle\"}" >/dev/null || fail "could not invite @$handle"
hand_off cabalName "$name"
echo "$scenario: '$name' invites @$handle, B's other invites are declined"
