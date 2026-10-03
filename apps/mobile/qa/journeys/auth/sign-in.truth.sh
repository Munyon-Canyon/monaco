#!/usr/bin/env bash
set -euo pipefail

accounts="apps/mobile/qa/journeys/accounts.tsv"
privy_user_id="$(awk -F '\t' '
  $1 == "actor" {
    for (i = 1; i <= NF; i++) {
      if ($i == "actor") actor_column = i
      if ($i == "privy_user_id") privy_user_id_column = i
    }
    next
  }
  actor_column != "" && $actor_column == "A" { print $privy_user_id_column; exit }
' "$accounts")"

if [[ -z "$privy_user_id" ]]; then
  echo "missing Privy user ID for actor A" >&2
  exit 1
fi

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
