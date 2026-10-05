#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"

fail=0
check() {
  if [[ "$2" == "$3" ]]; then
    echo "ok: $1"
  else
    echo "$1: got '$2', want '$3'"
    fail=1
  fi
}

count() {
  qa_sql -v name="QA activity $run" <<<"SELECT count(*) FROM cabal_activity a JOIN cabals c ON c.id = a.cabal_id
    WHERE c.name = :'name' AND $1" || { echo "database cannot be reached" >&2; exit 2; }
}

cabals="$(qa_sql -v name="QA activity $run" <<<"SELECT count(*) FROM cabals WHERE name = :'name'")" ||
  { echo "database cannot be reached" >&2; exit 2; }
if [[ "$cabals" == 0 ]]; then
  echo "no cabal QA activity $run: the setup did not run"
  exit 1
fi
check "one cabal QA activity $run" "$cabals" 1
check "six activity rows" "$(count true)" 6
check "the confirmed buy is still confirmed" "$(count "a.kind = 'buy' AND a.status = 'confirmed'")" 1
check "the failed buy is still failed" "$(count "a.kind = 'buy' AND a.status = 'failed'")" 1

exit "$fail"
