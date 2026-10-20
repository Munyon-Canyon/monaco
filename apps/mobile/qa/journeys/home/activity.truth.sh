#!/usr/bin/env bash
# Opening Activity writes nothing. This checks A is still the signed-in member the run started with.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
# shellcheck source=/dev/null
source scripts/qa/seed.sh

did="$(apps/mobile/qa/journeys/privy-user-id.sh A)"
row="$(qa_sql -v did="$did" <<<"SELECT auth_state || ' ' || (handle IS NOT NULL)::text FROM users WHERE privy_user_id = :'did'")" ||
  { echo "database cannot be reached" >&2; exit 2; }
if [[ "$row" != "ONBOARDING_COMPLETED true" ]]; then
  echo "A's users row reads '$row', not 'ONBOARDING_COMPLETED true'"
  exit 1
fi
echo "ok: A still has a handle and finished onboarding"
