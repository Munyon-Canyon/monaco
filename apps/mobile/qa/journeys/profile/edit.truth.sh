#!/usr/bin/env bash
set -euo pipefail

privy_user_id="$(apps/mobile/qa/journeys/privy-user-id.sh A)"
expected_name="Alfred ${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
query="SELECT display_name, photo_url IS NOT NULL FROM users WHERE privy_user_id = :'value'"

error_file="$(mktemp)"
trap 'rm -f "$error_file"' EXIT
set +e
row="$(printf '%s\n' "$query" | scripts/with-dotenv-local.sh apps/mobile/qa/journeys/psql.sh \
  -v ON_ERROR_STOP=1 -v value="$privy_user_id" -tA -F $'\t' 2>"$error_file")"
status=$?
set -e

if [[ $status -eq 127 ]]; then
  grep -m1 "psql not found" "$error_file" >&2 || echo "psql not found (install it, or start Compose postgres)" >&2
  exit 2
fi

if [[ $status -ne 0 ]]; then
  error_line="$(awk '!/^with-dotenv-local:/ && !/injected env/ { print; exit }' "$error_file")"
  [[ -n "$error_line" ]] && echo "$error_line" >&2
  echo "database cannot be reached" >&2
  exit 2
fi

IFS=$'\t' read -r display_name has_photo <<<"$row"
if [[ "$display_name" != "$expected_name" ]]; then
  echo "actor A's display_name is '$display_name', not '$expected_name'"
  exit 1
fi
if [[ "$has_photo" != "t" ]]; then
  echo "actor A has no profile photo"
  exit 1
fi
echo "actor A is '$expected_name' with a profile photo"
