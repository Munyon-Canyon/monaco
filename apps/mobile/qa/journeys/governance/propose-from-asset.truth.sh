#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
# shellcheck disable=SC1091
source scripts/qa/seed.sh
a="$(apps/mobile/qa/journeys/privy-user-id.sh A)"
name="QA asset $run"

if [[ "$(qa_sql -v name="$name" <<<"SELECT count(*) FROM cabals WHERE name = :'name'")" != 1 ]]; then
  echo "skip: the run never created $name"
  exit 0
fi

got="$(qa_sql -F ' ' -v name="$name" -v a="$a" <<<"SELECT count(*), max(p.kind), regexp_replace(max(p.symbol), 'x$', ''), max(p.usdc_micros)
  FROM proposals p JOIN cabals c ON c.id = p.cabal_id JOIN users u ON u.id = p.proposer_id
  WHERE c.name = :'name' AND u.privy_user_id = :'a'")"
want="1 buy GOOGL 25000000"
if [[ "$got" != "$want" ]]; then
  echo "A's proposal in $name: got '$got', want '$want'"
  exit 1
fi
echo "ok: A proposed a buy of GOOGL in $name"
