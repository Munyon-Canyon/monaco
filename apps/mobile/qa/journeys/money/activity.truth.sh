#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
# shellcheck source=/dev/null
source scripts/qa/seed.sh
a="$(qa_user_id A)" || exit 2

seeded="$(qa_sql -v a="$a" -v sig="QA$run" <<'SQL'
SELECT count(*) FROM user_txns t
WHERE t.user_id = :'a' AND t.tx_signature = :'sig' AND t.kind = 'deposit' AND t.status = 'settled'
  AND (SELECT sum(e.amount) FROM user_txn_entries e WHERE e.txn_id = t.id AND e.account = 'wallet') = 1230000
SQL
)"
present="$(qa_sql -v a="$a" -v sig="QA$run" <<<"SELECT count(*) FROM user_txns WHERE user_id = :'a' AND tx_signature = :'sig'")"

if [[ "$present" == 0 ]]; then
  echo "ok: S1 did not run, no seeded deposit to check"
  exit 0
fi
if [[ "$seeded" == 1 ]]; then
  echo "ok: the seeded deposit is still settled for \$1.23"
  exit 0
fi
echo "the seeded deposit changed: want one settled \$1.23 deposit, got $seeded"
exit 1
