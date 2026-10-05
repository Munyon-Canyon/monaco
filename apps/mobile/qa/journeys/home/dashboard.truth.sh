#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"

fail=0
members="$(qa_sql -v name="QA home $run" <<<"SELECT CASE WHEN count(DISTINCT c.id) = 0 THEN 'none' ELSE count(m.user_id)::text END
  FROM cabals c LEFT JOIN cabal_members m ON m.cabal_id = c.id WHERE c.name = :'name'")" ||
  { echo "database cannot be reached" >&2; exit 2; }
case "$members" in
  none) echo "ok: the run never created QA home $run" ;;
  1) echo "ok: QA home $run still has A as its only member" ;;
  *) echo "QA home $run members: got $members, want 1"; fail=1 ;;
esac

did="$(_qa_account C privy_user_id)"
c_cabals="$(qa_sql -v did="$did" <<<"SELECT count(*) FROM cabal_members m JOIN users u ON u.id = m.user_id WHERE u.privy_user_id = :'did'")"
if [[ "$c_cabals" == 0 ]]; then
  echo "ok: C belongs to no cabal"
else
  echo "C belongs to $c_cabals cabal(s), want 0 after S2"
  fail=1
fi
exit "$fail"
