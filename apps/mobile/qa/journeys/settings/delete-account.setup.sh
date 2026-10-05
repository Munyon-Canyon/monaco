#!/usr/bin/env bash
# journey.py runs this before each scenario. It makes a throwaway dev user at ONBOARDING_COMPLETED (P1),
# and before S3 a cabal that user alone belongs to (P3). No Privy test login is ever deleted.
set -euo pipefail

scenario="${1:?usage: delete-account.setup.sh <scenario>}"
fail() {
  echo "$scenario: $*" >&2
  exit 1
}

if [[ "${DELETE_ACCOUNT_SETUP_ENV:-}" != 1 ]]; then
  DELETE_ACCOUNT_SETUP_ENV=1 exec scripts/with-dotenv-local.sh "$0" "$@" 2> >(grep -v -e '^with-dotenv-local:' -e 'injected env' >&2)
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
  local method="$1" path="$2" body="$3"
  curl -fsS -X "$method" "$api$path" -H "Authorization: Bearer $token" -H 'Content-Type: application/json' \
    -H "Idempotency-Key: $(uuidgen | tr '[:upper:]' '[:lower:]')" -d "$body"
}

[[ -x bin/monacoctl ]] || fail "bin/monacoctl is missing: run just build backend"
log="$(mktemp)"
trap 'rm -f "$log"' EXIT
unset_qa=()
while read -r name; do unset_qa+=(-u "$name"); done < <(compgen -e | grep '^MONACO_QA_')
token="$(env "${unset_qa[@]}" bin/monacoctl dev token --user new 2>"$log")" ||
  fail "monacoctl dev token --user new failed: $(cat "$log")"
user_id="$(awk '$1 == "dev" && $2 == "user" { print $3 }' "$log")"
[[ -n "$user_id" ]] || fail "monacoctl dev token did not name the new dev user: $(cat "$log")"

updated="$(sql -v id="$user_id" <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED',
  auth_state_changed_at = now() WHERE id = :'id' RETURNING handle")"
handle="$(head -1 <<<"$updated")"
[[ -n "$handle" ]] || fail "dev user $user_id has no handle to keep after the delete"
call PATCH /v1/me "{\"display_name\":\"QA delete $run\"}" >/dev/null || fail "could not name dev user $user_id"
hand_off devToken "$token"
hand_off devUserID "$user_id"
hand_off "devUser$scenario" "$user_id"

if [[ "$scenario" == S3 ]]; then
  cabal_name="QA delete pot $run"
  cabal_id="$(call POST /v1/cabals \
    "{\"name\":\"$cabal_name\",\"join_mode\":\"open\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" |
    python3 -c 'import json, sys; print(json.load(sys.stdin)["id"])')" || fail "could not create '$cabal_name'"
  hand_off cabalID "$cabal_id"
  hand_off cabalName "$cabal_name"
  echo "$scenario: dev user $user_id (@$handle) alone in '$cabal_name'"
else
  echo "$scenario: dev user $user_id (@$handle) with no cabal and no balance"
fi
