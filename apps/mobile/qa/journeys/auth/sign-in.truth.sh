#!/usr/bin/env bash
set -euo pipefail

privy_user_id="$(apps/mobile/qa/journeys/privy-user-id.sh A)"
query="SELECT id FROM users WHERE privy_user_id = :'value' LIMIT 1"

error_file="$(mktemp)"
trap 'rm -f "$error_file"' EXIT
set +e
user_id="$(printf '%s\n' "$query" | scripts/with-dotenv-local.sh apps/mobile/qa/journeys/psql.sh \
  -v ON_ERROR_STOP=1 -v value="$privy_user_id" -tA 2>"$error_file")"
status=$?
set -e

if [[ $status -eq 127 ]]; then
  grep -m1 "psql not found" "$error_file" >&2 || echo "psql not found (install it, or start Compose postgres)" >&2
  exit 2
fi

if [[ $status -ne 0 ]]; then
  error_line="$(awk '!/^with-dotenv-local:/ && !/^⟐ injected env/ && !/^injected env/ { print; exit }' "$error_file")"
  [[ -n "$error_line" ]] && echo "$error_line" >&2
  echo "database cannot be reached" >&2
  exit 2
fi

if [[ -n "$user_id" ]]; then
  echo "found users row for actor A"
  exit 0
fi

echo "no users row for actor A"
exit 1
