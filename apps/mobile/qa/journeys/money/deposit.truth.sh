#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
# shellcheck source=/dev/null
source scripts/qa/seed.sh
a="$(qa_user_id A)" || exit 2

settled="$(qa_sql -v a="$a" <<<"SELECT count(*) FROM user_txns WHERE user_id = :'a' AND kind = 'deposit' AND status = 'settled'")"
failed="$(qa_sql -v a="$a" <<<"SELECT count(*) FROM user_txns WHERE user_id = :'a' AND kind = 'deposit' AND status = 'failed' AND created_at > now() - interval '1 hour'")"

fail=0
if [[ "$settled" -ge 1 ]]; then echo "ok: A has a settled deposit"; else echo "A has no settled deposit: fund A first"; fail=1; fi
if [[ "$failed" == 0 ]]; then echo "ok: no failed deposit this run"; else echo "A has $failed failed deposits in the last hour"; fail=1; fi
exit "$fail"
