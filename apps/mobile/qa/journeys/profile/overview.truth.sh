#!/usr/bin/env bash
set -euo pipefail

name="QA overview ${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
did="$(apps/mobile/qa/journeys/privy-user-id.sh A)"
query="SELECT u.auth_state, EXISTS (SELECT 1 FROM cabal_members m JOIN cabals c ON c.id = m.cabal_id
  WHERE m.user_id = u.id AND c.name = :'name') FROM users u WHERE u.privy_user_id = :'did'"

error_file="$(mktemp)"
trap 'rm -f "$error_file"' EXIT
if ! row="$(printf '%s\n' "$query" | scripts/with-dotenv-local.sh apps/mobile/qa/journeys/psql.sh \
  -v ON_ERROR_STOP=1 -v did="$did" -v name="$name" -tA -F $'\t' 2>"$error_file")"; then
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
  echo "actor A is not a member of '$name'"
  exit 1
fi
echo "actor A is ONBOARDING_COMPLETED and still a member of '$name'"
