#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"
a="$("$QA_ROOT/apps/mobile/qa/journeys/privy-user-id.sh" A)"

fail=0
check() {
  if [[ "$2" == "$3" ]]; then
    echo "ok: $1"
  else
    echo "$1: got '$2', want '$3'"
    fail=1
  fi
}

query() {
  qa_sql -v name="QA cash out $run" -v a="$a" || { echo "database cannot be reached" >&2; exit 2; }
}

if [[ "$(query <<<"SELECT count(*) FROM cabals WHERE name = :'name'")" == 0 ]]; then
  echo "skip: no cabal QA cash out $run, S1 did not run"
  exit "$fail"
fi
check "A is still a member of QA cash out $run" "$(query <<<"SELECT count(*) FROM cabal_members m
  JOIN cabals c ON c.id = m.cabal_id JOIN users u ON u.id = m.user_id
  WHERE c.name = :'name' AND u.privy_user_id = :'a'")" 1
check "no cash out reached QA cash out $run (#657)" "$(query <<<"SELECT count(*) FROM cabal_activity x
  JOIN cabals c ON c.id = x.cabal_id WHERE c.name = :'name' AND x.kind = 'cash_out'")" 0

exit "$fail"
