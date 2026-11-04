#!/usr/bin/env bash
# journey.py runs this before each scenario with the scenario id. It sets actor A's auth_state to the
# one the scenario starts from (Preconditions P3), so no one edits the database by hand.
set -euo pipefail

: "${1:?usage: edit.setup.sh <scenario>}"
# The photos precondition: the simulator holds the fixture photo the picker step chooses.
# shellcheck source=/dev/null
source scripts/qa/seed.sh
photo_marker="${TMPDIR:-/tmp}/monaco-qa-photo-${MONACO_QA_RUN:?}-${MONACO_QA_SIM_UDID:?journey.py sets MONACO_QA_SIM_UDID}"
if [[ ! -e "$photo_marker" ]]; then
  qa_add_photo "$MONACO_QA_SIM_UDID"
  touch "$photo_marker"
fi
state=AWAITING_SOCIALS

privy_user_id="$(apps/mobile/qa/journeys/privy-user-id.sh A)"
query="UPDATE users SET auth_state = :'state', auth_state_changed_at = now() WHERE privy_user_id = :'privy_user_id'"

error_file="$(mktemp)"
trap 'rm -f "$error_file"' EXIT
updated="$(printf '%s\n' "$query" | scripts/with-dotenv-local.sh apps/mobile/qa/journeys/psql.sh \
  -v ON_ERROR_STOP=1 -v state="$state" -v privy_user_id="$privy_user_id" -tA 2>"$error_file")" || true

if [[ "$updated" != "UPDATE 1" ]]; then
  grep -v -e '^with-dotenv-local:' -e 'injected env' "$error_file" >&2 || true
  echo "$1: could not set actor A to $state (got '${updated}'). A has no users row until auth/sign-in has run once" >&2
  exit 1
fi
echo "$1: actor A is $state"
