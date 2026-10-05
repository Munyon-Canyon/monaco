#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
# shellcheck source=/dev/null
source scripts/qa/seed.sh
a="$(qa_user_id A)" || exit 2
name="QA fund $run"

member="$(qa_sql -v a="$a" -v name="$name" <<<"SELECT count(*) FROM cabal_members m JOIN cabals c ON c.id = m.cabal_id
  WHERE c.name = :'name' AND m.user_id = :'a'")"
odd="$(qa_sql -v a="$a" -v name="$name" <<'SQL'
SELECT count(*) FROM user_txns t JOIN cabals c ON c.id = t.cabal_id
WHERE c.name = :'name' AND t.user_id = :'a' AND t.kind = 'fund'
  AND (t.status = 'failed' OR (SELECT sum(e.amount) FROM user_txn_entries e WHERE e.txn_id = t.id AND e.account = 'wallet') <> -1000000)
SQL
)"

fail=0
if [[ "$member" == 1 ]]; then echo "ok: A is a member of $name"; else echo "A is not a member of $name"; fail=1; fi
if [[ "$odd" == 0 ]]; then echo "ok: every fund into $name is \$1 and not failed"; else echo "$odd fund rows into $name are failed or not \$1"; fail=1; fi
exit "$fail"
