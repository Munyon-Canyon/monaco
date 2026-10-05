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

if [[ "$(exists "QA leave $run")" == 1 ]]; then
  check "B left QA leave $run" "$(member "QA leave $run" b)" 0
fi
if [[ "$(exists "QA stay $run")" == 1 ]]; then
  check "A is still in QA stay $run" "$(member "QA stay $run" a)" 1
  check "B is still in QA stay $run" "$(member "QA stay $run" b)" 1
fi

exit "$fail"
