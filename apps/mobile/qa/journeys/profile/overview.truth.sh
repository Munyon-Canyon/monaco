#!/usr/bin/env bash
set -euo pipefail

did="$(apps/mobile/qa/journeys/privy-user-id.sh A)"
query="SELECT u.auth_state, EXISTS (SELECT 1 FROM cabal_members m WHERE m.user_id = u.id) FROM users u WHERE u.privy_user_id = :'did'"

error_file="$(mktemp)"
trap 'rm -f "$error_file"' EXIT
if ! row="$(printf '%s\n' "$query" | scripts/with-dotenv-local.sh apps/mobile/qa/journeys/psql.sh \
  -v ON_ERROR_STOP=1 -v did="$did" -tA -F $'\t' 2>"$error_file")"; then
  grep -v -e '^with-dotenv-local:' -e 'injected env' "$error_file" >&2 || true
  echo "database cannot be reached" >&2
  exit 2
fi

IFS=$'\t' read -r state member <<<"$row"
if [[ "$state" != ONBOARDING_COMPLETED ]]; then
  echo "actor A's auth_state is '$state', not ONBOARDING_COMPLETED"
  exit 1
fi
if [[ "$member" != t ]]; then
  echo "actor A is not a member of any cabal"
  exit 1
fi
echo "actor A is ONBOARDING_COMPLETED and still a member of a cabal"
