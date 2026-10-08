#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
b=""
a="$(apps/mobile/qa/journeys/privy-user-id.sh A)"

sql() {
  scripts/with-dotenv-local.sh apps/mobile/qa/journeys/psql.sh -v ON_ERROR_STOP=1 -tA -F ' ' \
    -v run="$run" -v a="$a" -v b="$b" 2> >(grep -v -e '^with-dotenv-local:' -e 'injected env' >&2) ||
    { echo "database cannot be reached" >&2; exit 2; }
}

fail=0
check() {
  if [[ "$2" == "$3" ]]; then
    echo "ok: $1"
  else
    echo "$1: got '$2', want '$3'"
    fail=1
  fi
}

pending="$(sql <<<"SELECT count(*) FROM cabal_access_requests r JOIN cabals c ON c.id = r.cabal_id
  JOIN users u ON u.id = r.user_id WHERE c.name = 'QA ask $run' AND u.privy_user_id = :'a' AND r.status = 'pending'")"
check "A asked to join QA ask $run" "$pending" 1

exit "$fail"
