#!/usr/bin/env bash
# journey.py runs this before each scenario with the scenario id (docs/journeys/onboarding/first-run.md,
# Preconditions). S1 makes actor C a new member, frees {L.phone} and hands the test a taken handle.
# S2 to S4 put C where S1 leaves it, so each also runs alone. S5 makes a new dev user at
# AWAITING_SOCIALS and hands the test its token.
set -euo pipefail

scenario="${1:?usage: first-run.setup.sh <scenario>}"

fail() {
  echo "$scenario: $*" >&2
  exit 1
}

if [[ "${FIRST_RUN_SETUP_ENV:-}" != 1 ]]; then
  FIRST_RUN_SETUP_ENV=1 exec scripts/with-dotenv-local.sh "$0" "$@" 2> >(grep -v -e '^with-dotenv-local:' -e 'injected env' >&2)
fi

: "${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"

sql() {
  apps/mobile/qa/journeys/psql.sh -v ON_ERROR_STOP=1 -tA "$@"
}

privy() {
  curl -sS -u "$PRIVY_APP_ID:$PRIVY_APP_SECRET" -H "privy-app-id: $PRIVY_APP_ID" \
    -H 'Content-Type: application/json' "$@"
}

privy_user_by() {
  local path="$1" body="$2" response status
  response="$(privy -w $'\n%{http_code}' -X POST "https://auth.privy.io/api/v1/users/$path" -d "$body")"
  status="${response##*$'\n'}"
  case "$status" in
    200) printf '%s' "${response%$'\n'*}" ;;
    404) ;;
    *) fail "Privy answered $status looking up a user by $path" ;;
  esac
}

json_field() {
  python3 -c 'import json, sys; print(json.load(sys.stdin).get(sys.argv[1], ""))' "$1"
}

linked_phone() {
  python3 -c '
import json, sys
for account in json.load(sys.stdin).get("linked_accounts", []):
    if account.get("type") == "phone":
        print(account.get("number") or account.get("phoneNumber"))
        break
'
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

free_link_number() {
  local member_id="$1" number="+1${MONACO_QA_L_PHONE:?}" holder holder_id holder_handle
  holder="$(privy_user_by phone/number "{\"number\":\"$number\"}")"
  [[ -n "$holder" ]] || return 0
  holder_id="$(json_field id <<<"$holder")"
  if [[ "$holder_id" == "$member_id" ]]; then
    privy -f -X POST "https://auth.privy.io/api/v1/apps/$PRIVY_APP_ID/users/unlink" \
      -d "{\"user_id\":\"$holder_id\",\"type\":\"phone\",\"handle\":\"$(linked_phone <<<"$holder")\"}" > /dev/null ||
      fail "could not unlink {L.phone} from actor C's Privy user $holder_id"
  else
    holder_handle="$(sql -v id="$holder_id" <<<"SELECT handle FROM users WHERE privy_user_id = :'id'")"
    [[ "$holder_handle" == dev_* ]] ||
      fail "{L.phone} belongs to Privy user $holder_id (handle '${holder_handle}'), which is neither actor C nor a dev user"
    privy -f -X DELETE "https://auth.privy.io/api/v1/users/$holder_id" > /dev/null ||
      fail "could not delete dev user $holder_handle's Privy user $holder_id"
  fi
  sql -v id="$holder_id" > /dev/null <<<"UPDATE users SET phone_e164 = NULL, phone_hash = NULL,
    phone_verified_at = NULL WHERE privy_user_id = :'id'"
  echo "$scenario: freed {L.phone} from Privy user $holder_id"
}

member_privy_id() {
  local member
  member="$(privy_user_by email/address "{\"address\":\"${MONACO_QA_C_EMAIL:?}\"}")"
  [[ -z "$member" ]] || json_field id <<<"$member"
}

set_up_first_run() {
  local member_id taken
  member_id="$(member_privy_id)"
  free_link_number "$member_id"
  if [[ -n "$member_id" ]]; then
    sql -v id="$member_id" > /dev/null <<<"UPDATE users SET handle = NULL, handle_changed_at = NULL,
      auth_state = 'CREATED', auth_state_changed_at = now(), phone_e164 = NULL, phone_hash = NULL,
      phone_verified_at = NULL WHERE privy_user_id = :'id'"
  fi
  echo "$scenario: actor C is a new member"

  taken="$(sql -v id="$(apps/mobile/qa/journeys/privy-user-id.sh A)" \
    <<<"SELECT handle FROM users WHERE privy_user_id = :'id'")"
  [[ -n "$taken" ]] || fail "actor A has no handle to use as a taken one. Run auth/sign-in first"
  hand_off takenHandle "$taken"
  echo "$scenario: handed the test @$taken as a taken handle"
}

set_up_after_first_run() {
  local member_id updated
  member_id="$(member_privy_id)"
  [[ -n "$member_id" ]] || fail "actor C has never signed in by email. Run S1 first"
  free_link_number "$member_id"
  updated="$(sql -v id="$member_id" <<<"UPDATE users SET handle = 'qa_cayman', auth_state = 'AWAITING_PHONE',
    auth_state_changed_at = now(), phone_e164 = NULL, phone_hash = NULL, phone_verified_at = NULL
    WHERE privy_user_id = :'id'")"
  [[ "$updated" == "UPDATE 1" ]] || fail "actor C has no users row. Run S1 first"
  echo "$scenario: actor C is @qa_cayman at AWAITING_PHONE with no phone"
}

set_up_dev_user() {
  local token user_id log updated unset_qa=(-u MONACO_API_BASE_URL)
  [[ -x bin/monacoctl ]] || fail "bin/monacoctl is missing: run just build backend"
  log="$(mktemp)"
  trap 'rm -f "$log"' RETURN
  while read -r name; do unset_qa+=(-u "$name"); done < <(compgen -e | grep '^MONACO_QA_')
  token="$(env "${unset_qa[@]}" bin/monacoctl dev token --user new 2> "$log")" ||
    fail "monacoctl dev token --user new failed: $(cat "$log")"
  user_id="$(awk '$1 == "dev" && $2 == "user" { print $3 }' "$log")"
  [[ -n "$user_id" ]] || fail "monacoctl dev token did not name the new dev user: $(cat "$log")"
  updated="$(sql -v id="$user_id" <<<"UPDATE users SET auth_state = 'AWAITING_SOCIALS',
    auth_state_changed_at = now() WHERE id = :'id' AND auth_state = 'AWAITING_PHONE'")"
  [[ "$updated" == "UPDATE 1" ]] || fail "could not move dev user $user_id to AWAITING_SOCIALS (got '$updated')"
  hand_off devToken "$token"
  hand_off devUserID "$user_id"
  echo "$scenario: dev user $user_id is AWAITING_SOCIALS"
}

case "$scenario" in
  S1) set_up_first_run ;;
  S2 | S3 | S4) set_up_after_first_run ;;
  S5) set_up_dev_user ;;
  *) fail "no setup for this scenario" ;;
esac
