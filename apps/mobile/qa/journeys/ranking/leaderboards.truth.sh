#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"

got="$(qa_sql -v name="QA ranks $run" <<<"SELECT CASE WHEN count(DISTINCT c.id) = 0 THEN 'none'
    ELSE count(m.user_id)::text || ' ' || count(m.user_id) FILTER (WHERE m.role = 'creator')::text END
  FROM cabals c LEFT JOIN cabal_members m ON m.cabal_id = c.id WHERE c.name = :'name'")" ||
  { echo "database cannot be reached" >&2; exit 2; }
case "$got" in
  none) echo "ok: the run never created QA ranks $run" ;;
  "3 1") echo "ok: QA ranks $run has three members, one creator" ;;
  *) echo "QA ranks $run members and creators: got '$got', want '3 1'"; exit 1 ;;
esac
