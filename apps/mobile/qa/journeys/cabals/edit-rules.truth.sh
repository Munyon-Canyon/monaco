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



rules="$(sql <<<"SELECT threshold, proposal_expiry_seconds, voter_mode FROM cabals WHERE name = 'QA renamed $run'")"
check "QA rules $run was renamed and its rules saved" "$rules" "unanimous 604800 list"
voters="$(sql <<<"SELECT count(*) FROM cabal_members m JOIN cabals c ON c.id = m.cabal_id
  WHERE c.name = 'QA renamed $run' AND m.can_vote")"
check "voters of QA renamed $run" "$voters" 2

exit "$fail"
