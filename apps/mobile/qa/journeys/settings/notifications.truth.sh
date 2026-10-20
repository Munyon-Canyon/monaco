#!/usr/bin/env bash
set -euo pipefail

name="QA push ${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
did="$(apps/mobile/qa/journeys/privy-user-id.sh B)"
query="SELECT EXISTS (SELECT 1 FROM cabal_members m JOIN cabals c ON c.id = m.cabal_id
  JOIN users u ON u.id = m.user_id WHERE u.privy_user_id = :'did' AND c.name = :'name')"

error_file="$(mktemp)"
trap 'rm -f "$error_file"' EXIT
if ! member="$(printf '%s\n' "$query" | scripts/with-dotenv-local.sh apps/mobile/qa/journeys/psql.sh \
  -v ON_ERROR_STOP=1 -v did="$did" -v name="$name" -tA 2>"$error_file")"; then
  grep -v -e '^with-dotenv-local:' -e 'injected env' "$error_file" >&2 || true
  echo "database cannot be reached" >&2
  exit 2
fi
if [[ "$member" != t ]]; then
  echo "actor B is not a member of '$name'"
  exit 1
fi
echo "actor B joined '$name'"

query="SELECT COALESCE((SELECT enabled FROM notification_preferences p JOIN users u ON u.id = p.user_id
  WHERE u.privy_user_id = :'did' AND p.category = 'decision_requests'), true),
  COALESCE((SELECT enabled FROM notification_preferences p JOIN users u ON u.id = p.user_id
  WHERE u.privy_user_id = :'did' AND p.category = 'replies_and_mentions'), true)"
if ! choices="$(printf '%s\n' "$query" | scripts/with-dotenv-local.sh apps/mobile/qa/journeys/psql.sh \
  -v ON_ERROR_STOP=1 -v did="$did" -tA 2>"$error_file")"; then
  grep -v -e '^with-dotenv-local:' -e 'injected env' "$error_file" >&2 || true
  echo "database cannot be reached" >&2
  exit 2
fi
if [[ "$choices" != "f|t" ]]; then
  echo "actor B's choices are '$choices', not decision_requests off and replies_and_mentions on (f|t)"
  exit 1
fi
echo "actor B turned Decision requests off and left Replies and mentions on"
