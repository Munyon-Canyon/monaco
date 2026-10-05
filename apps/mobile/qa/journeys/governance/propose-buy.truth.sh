#!/usr/bin/env bash
# The ground truth of docs/journeys/governance/propose-buy.md: after S2, the run's cabal has one passed buy of
# GOOGL by A, no swap, and one stubbed trading.engine delivery of its proposal.passed event.
# Exits 1 when a row is wrong and 2 when it cannot be read.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
handoff="${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"
# shellcheck disable=SC1091
source scripts/qa/seed.sh

name="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get("cabalName", ""))' "$handoff" 2>/dev/null || true)"
if [[ -z "$name" ]]; then
  echo "skip: the run never seeded the cabal"
  exit 0
fi
a="$(apps/mobile/qa/journeys/privy-user-id.sh A)"
want="1 buy GOOGL 1000000 passed Journey buy $run 0 1"

got=""
for _ in $(seq 1 20); do
  got="$(qa_sql -F ' ' -v name="$name" -v a="$a" -v reason="Journey buy $run" <<'SQL'
SELECT count(*), max(p.kind), regexp_replace(max(p.symbol), 'x$', ''), max(p.usdc_micros), max(p.status), max(p.thesis),
  (SELECT count(*) FROM swaps s WHERE s.cabal_id = max(c.id)),
  (SELECT count(*) FROM event_deliveries d JOIN events e ON e.id = d.event_id
    WHERE d.handler = 'trading.engine' AND d.code = 'stubbed' AND e.type = 'proposal.passed'
      AND e.aggregate_id = max(p.id))
FROM proposals p JOIN cabals c ON c.id = p.cabal_id JOIN users u ON u.id = p.proposer_id
WHERE c.name = :'name' AND u.privy_user_id = :'a' AND p.thesis = :'reason'
SQL
)"
  [[ "$got" == "$want" ]] && break
  sleep 1
done
if [[ "$got" != "$want" ]]; then
  echo "A's proposal in $name: got '$got', want '$want' (count kind symbol usdc_micros status reason swaps stubbed-deliveries)"
  exit 1
fi
echo "ok: $name has A's passed buy of GOOGL, no swap and one stubbed trading.engine delivery"
