#!/usr/bin/env bash
# The ground truth of docs/journeys/governance/propose-buy.md, read after the whole run: A's proposal is an executed
# buy of GOOGL with exactly one confirmed buy swap and one confirmed buy in cabal_activity. After the refund the
# cabal holds none of that asset, A and B hold no shares, and both have a settled cash out. The cash outs sell the
# GOOGL again, so the cabal's swaps and positions are read per proposal and per asset, never as a total.
# Exits 1 when a row is wrong and 2 when it cannot be read.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
handoff="${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"
# shellcheck disable=SC1091
source scripts/qa/seed.sh

name="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get("cabalName", ""))' "$handoff" 2>/dev/null || true)"
if [[ -z "$name" ]]; then
  echo "skip: the run never made the cabal"
  exit 0
fi
a="$(apps/mobile/qa/journeys/privy-user-id.sh A)"
b="$(apps/mobile/qa/journeys/privy-user-id.sh B)"

query() {
  qa_sql -F ' ' -v name="$name" -v a="$a" -v b="$b" -v reason="Journey buy $run" || {
    echo "database cannot be reached" >&2
    exit 2
  }
}

want="1 buy GOOGL 1000000 executed Journey buy $run 1 confirmed 1 0 0 2"
got=""
for _ in $(seq 1 20); do
  got="$(query <<'SQL'
WITH mine AS (
  SELECT p.id, p.cabal_id, p.kind, p.symbol, p.usdc_micros, p.status, p.thesis
  FROM proposals p JOIN cabals c ON c.id = p.cabal_id JOIN users u ON u.id = p.proposer_id
  WHERE c.name = :'name' AND u.privy_user_id = :'a' AND p.thesis = :'reason'
)
SELECT count(*), max(kind), regexp_replace(max(symbol), 'x$', ''), max(usdc_micros), max(status), max(thesis),
  (SELECT count(*) FROM swaps s WHERE s.source_kind = 'proposal' AND s.source_id IN (SELECT id FROM mine) AND s.action = 'buy'),
  coalesce((SELECT max(s.status) FROM swaps s WHERE s.source_kind = 'proposal' AND s.source_id IN (SELECT id FROM mine) AND s.action = 'buy'), 'none'),
  (SELECT count(*) FROM cabal_activity x JOIN swaps s ON s.id = x.id
    WHERE s.source_kind = 'proposal' AND s.source_id IN (SELECT id FROM mine) AND x.kind = 'buy'
      AND x.status = 'confirmed' AND x.asset = s.out_mint),
  (SELECT coalesce(sum(cp.units), 0) FROM cabal_positions cp JOIN swaps s ON s.cabal_id = cp.cabal_id AND s.out_mint = cp.asset
    WHERE s.source_kind = 'proposal' AND s.source_id IN (SELECT id FROM mine)),
  (SELECT coalesce(sum(up.share_units), 0) FROM user_positions up JOIN users u ON u.id = up.user_id
    WHERE up.cabal_id IN (SELECT cabal_id FROM mine) AND u.privy_user_id IN (:'a', :'b')),
  (SELECT count(DISTINCT t.user_id) FROM user_txns t JOIN users u ON u.id = t.user_id
    WHERE t.cabal_id IN (SELECT cabal_id FROM mine) AND t.kind = 'cash_out' AND t.status = 'settled'
      AND u.privy_user_id IN (:'a', :'b'))
FROM mine
SQL
)"
  [[ "$got" == "$want" ]] && break
  sleep 1
done
if [[ "$got" != "$want" ]]; then
  echo "A's proposal in $name: got '$got', want '$want' (count kind symbol usdc_micros status reason buy-swaps swap-status confirmed-buy-activity cabal-units-left share-units-of-A-and-B settled-cash-out-users)"
  exit 1
fi
echo "ok: $name has A's executed buy of GOOGL, one confirmed swap, no GOOGL or shares left after the refund and a settled cash out for A and B"
