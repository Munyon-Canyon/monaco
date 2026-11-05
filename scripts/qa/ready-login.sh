#!/usr/bin/env bash
# Makes the login a run holds for an actor a finished member, so the first-run gate opens the tab bar
# (docs/journeys/auth/sign-in.md P4). journey.py runs it for each actor before the journey's setup, with
# QA_ACCOUNTS_FILE pointing at the run's accounts, so a doc actor remapped onto another login prepares that
# login. Idempotent: a member that already has a handle and an auth_state past CREATED is left alone.
#
#   scripts/qa/ready-login.sh <actor>
#
# A login with no users row yet, as on a fresh slot database, signs in first the way the app does: Privy's
# passwordless route with the login's fixed test code gives an access token, and POST /v1/auth/session opens the
# backend session, which creates the row and the wallet. So every actor the journey lists is a member before setup
# runs, also one that first acts in a later scenario. The handle is qa_<name>; when another user already holds it (handle_taken), it is
# qa_<name>_<the first 6 characters of the login's Privy id after did:privy:>, which belongs to this login alone.
set -euo pipefail

# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"

qa_handle() {
  tr '[:upper:]' '[:lower:]' <<<"qa_${1//[^[:alnum:]]/}"
}

qa_handle_fallback() {
  local suffix="${2#did:privy:}"
  echo "$(qa_handle "$1")_${suffix:0:6}"
}

qa_privy_ids() {
  (cd "$QA_ROOT" && scripts/with-dotenv-local.sh env 2> >(_qa_quiet)) | grep -E '^PRIVY_APP_(ID|CLIENT_ID)='
}

qa_privy_post() {
  curl -fsS --max-time 20 -X POST "https://auth.privy.io/api/v1/$1" -H "Content-Type: application/json" \
    -H "privy-app-id: $QA_PRIVY_APP_ID" -H "privy-client-id: $QA_PRIVY_CLIENT_ID" \
    -H "x-native-app-identifier: xyz.monacolabs.app" -d "$2"
}

# qa_first_sign_in <actor>: opens the login's first backend session, which creates its users row.
qa_first_sign_in() {
  local actor="$1" phone email code route field value ids token QA_PRIVY_APP_ID QA_PRIVY_CLIENT_ID
  phone="$(_qa_account "$actor" phone)"
  email="$(_qa_account "$actor" email)"
  code="$(_qa_account "$actor" code)"
  if [[ -n "$phone" ]]; then
    route=passwordless_sms field=phoneNumber value="+1 $phone"
  else
    route=passwordless field=email value="$email"
  fi
  ids="$(qa_privy_ids)"
  QA_PRIVY_APP_ID="$(sed -n 's/^PRIVY_APP_ID=//p' <<<"$ids")"
  QA_PRIVY_CLIENT_ID="$(sed -n 's/^PRIVY_APP_CLIENT_ID=//p' <<<"$ids")"
  qa_privy_post "$route/init" "{\"$field\":\"$value\"}" >/dev/null
  token="$(qa_privy_post "$route/authenticate" "{\"$field\":\"$value\",\"code\":\"$code\"}" |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')"
  curl -fsS --max-time 30 -X POST "$QA_API/v1/auth/session" -H "Authorization: Bearer $token" >/dev/null
}

ready_login() {
  local actor="${1:?usage: ready-login.sh <actor>}" did name row handle state err
  qa_api_ready
  did="$(_qa_account "$actor" privy_user_id)"
  name="$(_qa_account "$actor" name)"
  [[ -n "$did" ]] || { echo "ready-login: actor $actor has no privy_user_id in $QA_ACCOUNTS" >&2; return 1; }
  row="$(qa_sql -v did="$did" <<<"SELECT coalesce(handle, ''), auth_state FROM users WHERE privy_user_id = :'did'")"
  if [[ -z "$row" ]]; then
    qa_first_sign_in "$actor"
    echo "ready-login: actor $actor ($did) had no users row: signed in once"
    row="$(qa_sql -v did="$did" <<<"SELECT coalesce(handle, ''), auth_state FROM users WHERE privy_user_id = :'did'")"
    [[ -n "$row" ]] || { echo "ready-login: actor $actor still has no users row after signing in" >&2; return 1; }
  fi
  IFS='|' read -r handle state <<<"$row"

  if [[ -z "$handle" ]]; then
    handle="$(qa_handle "$name")"
    if ! err="$(qa_api "$actor" PUT /v1/me/handle "{\"handle\":\"$handle\"}" 2>&1 >/dev/null)"; then
      if [[ "$err" != *handle_taken* ]]; then
        echo "$err" >&2
        return 1
      fi
      handle="$(qa_handle_fallback "$name" "$did")"
      qa_api "$actor" PUT /v1/me/handle "{\"handle\":\"$handle\"}" >/dev/null
    fi
    echo "ready-login: actor $actor got the handle $handle"
  fi
  if [[ "$state" == CREATED ]]; then
    # AWAITING_PHONE is past CREATED: the app opens the tab bar and shows the add-a-number banner
    # (docs/architecture/auth.md#auth_state). Skipping the phone step is a route, so no SQL.
    qa_api "$actor" POST /v1/me/onboarding/skip '{"step":"phone"}' >/dev/null
    state=AWAITING_PHONE
  fi
  echo "ready-login: actor $actor is @$handle, $state"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  ready_login "$@"
fi
