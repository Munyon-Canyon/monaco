#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
b="$(apps/mobile/qa/journeys/privy-user-id.sh B)"
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

member() {
  sql <<<"SELECT count(*) FROM cabal_members m JOIN cabals c ON c.id = m.cabal_id JOIN users u ON u.id = m.user_id
    WHERE c.name = '$1' AND u.privy_user_id = :'$2'"
}

exists() {
  sql <<<"SELECT count(*) FROM cabals WHERE name = '$1'"
}

requests() {
  sql <<<"SELECT string_agg(r.status, ',' ORDER BY r.created_at) FROM cabal_access_requests r
    JOIN cabals c ON c.id = r.cabal_id JOIN users u ON u.id = r.user_id
    WHERE c.name = '$1' AND u.privy_user_id = :'b' AND r.direction = 'request'"
}

if [[ "$(exists "QA ask $run")" == 1 ]]; then
  check "B's requests to QA ask $run" "$(requests "QA ask $run")" "revoked,denied"
  check "B is not in QA ask $run" "$(member "QA ask $run" b)" 0
fi
if [[ "$(exists "QA ok $run")" == 1 ]]; then
  check "B's request to QA ok $run" "$(requests "QA ok $run")" "approved"
  check "B is in QA ok $run" "$(member "QA ok $run" b)" 1
fi

exit "$fail"
