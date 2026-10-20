#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"

got="$(qa_sql -v name="QA info $run" <<<"SELECT CASE WHEN count(DISTINCT c.id) = 0 THEN 'none'
    ELSE count(m.user_id)::text || ' ' || count(m.user_id) FILTER (WHERE m.can_vote)::text END
  FROM cabals c LEFT JOIN cabal_members m ON m.cabal_id = c.id WHERE c.name = :'name'")" ||
  { echo "database cannot be reached" >&2; exit 2; }
case "$got" in
  none) echo "ok: the run never created QA info $run" ;;
  "2 2") echo "ok: QA info $run has two members and both vote after Save voters" ;;
  *) echo "QA info $run members and voters: got '$got', want '2 2'"; exit 1 ;;
esac
