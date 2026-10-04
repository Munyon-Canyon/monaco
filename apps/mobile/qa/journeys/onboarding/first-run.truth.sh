#!/usr/bin/env bash
# The ground truth of docs/journeys/onboarding/first-run.md: actor C's row after S1 to S4, and the S5 dev user's
# row, read through psql.sh. Exits 1 when a row is wrong and 2 when the database or Privy cannot be read.
set -euo pipefail

if [[ "${FIRST_RUN_TRUTH_ENV:-}" != 1 ]]; then
  FIRST_RUN_TRUTH_ENV=1 exec scripts/with-dotenv-local.sh "$0" "$@" 2> >(grep -v -e '^with-dotenv-local:' -e 'injected env' >&2)
fi

: "${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"

unreadable() {
  echo "$*" >&2
  exit 2
}

row() {
  local status=0 out
  out="$(apps/mobile/qa/journeys/psql.sh -v ON_ERROR_STOP=1 -tA -F '|' "$@")" || status=$?
  [[ $status -eq 127 ]] && unreadable "psql not found (install it, or start Compose postgres)"
  [[ $status -ne 0 ]] && unreadable "database cannot be reached"
  printf '%s' "$out"
}

member_id="$(curl -sS -f -u "$PRIVY_APP_ID:$PRIVY_APP_SECRET" -H "privy-app-id: $PRIVY_APP_ID" \
  -H 'Content-Type: application/json' -X POST https://auth.privy.io/api/v1/users/email/address \
  -d "{\"address\":\"${MONACO_QA_C_EMAIL:?}\"}" |
  python3 -c 'import json, sys; print(json.load(sys.stdin)["id"])')" ||
  unreadable "could not find actor C's Privy user by email"

dev_user_id="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get("devUserID", ""))' \
  "$MONACO_QA_HANDOFF" 2> /dev/null || true)"

failed=0
check() {
  if [[ "$2" != "$3" ]]; then
    echo "$1 is '$2', not '$3'"
    failed=1
  fi
}

IFS='|' read -r handle phone state <<<"$(row -v id="$member_id" \
  <<<"SELECT handle, phone_e164, auth_state FROM users WHERE privy_user_id = :'id'")"
check "actor C's handle" "${handle:-}" qa_cayman
check "actor C's phone" "${phone:-}" "+1${MONACO_QA_L_PHONE:?}"
check "actor C's auth_state" "${state:-}" AWAITING_SOCIALS

if [[ -z "$dev_user_id" ]]; then
  echo "the hand-off has no devUserID: S5's setup did not run"
  failed=1
else
  IFS='|' read -r x_dev state <<<"$(row -v id="$dev_user_id" \
    <<<"SELECT x_username LIKE 'dev\_x\_%', auth_state FROM users WHERE id = :'id'")"
  check "dev user $dev_user_id has a dev_x_ X account" "${x_dev:-}" t
  check "dev user $dev_user_id's auth_state" "${state:-}" ONBOARDING_COMPLETED
fi

[[ $failed -eq 0 ]] || exit 1
echo "actor C is @qa_cayman with {L.phone} at AWAITING_SOCIALS, and dev user $dev_user_id linked a dev_x_ account"
