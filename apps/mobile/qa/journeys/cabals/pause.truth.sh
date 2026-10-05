#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"

fail=0
open_pauses() {
  qa_sql -v name="$1" <<<"SELECT CASE WHEN count(DISTINCT c.id) = 0 THEN 'none' ELSE count(p.id)::text END
    FROM cabals c LEFT JOIN cabal_pauses p ON p.cabal_id = c.id AND p.resolved_at IS NULL WHERE c.name = :'name'" ||
    { echo "database cannot be reached" >&2; exit 2; }
}
check() {
  local got
  got="$(open_pauses "$1")"
  if [[ "$got" == none ]]; then
    echo "ok: the run never created $1"
  elif [[ "$got" == "$2" ]]; then
    echo "ok: $1 has $2 open pause(s)"
  else
    echo "$1 open pauses: got '$got', want '$2'"
    fail=1
  fi
}

check "QA paused $run" 1
check "QA running $run" 0
exit "$fail"
