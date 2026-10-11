#!/usr/bin/env bash
# The ground truth of docs/journeys/money/best-price-trade.md, read after the whole run: A's proposal is an executed
# buy of Apple with exactly one confirmed buy swap. That swap has a route whose comparison quoted at least two
# listings and named one of them the winner, and every step of the route succeeded. After the refund the cabal holds
# none of the asset it bought, and A has a settled cash out.
# Exits 1 when a row is wrong and 2 when it cannot be read.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
handoff="${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"
# shellcheck disable=SC1091
source scripts/qa/seed.sh

name="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get("cabalName", ""))' "$handoff" 2>/dev/null || true)"
if [[ -z "$name" ]]; then
  echo "skip: the run never made the cabal"
  exit 0
fi
a="$(apps/mobile/qa/journeys/privy-user-id.sh A)"

query() {
  qa_sql -F ' ' -v name="$name" -v a="$a" || {
    echo "database cannot be reached" >&2
    exit 2
  }
}

want="1 buy 1000000 executed 1 confirmed 1 t t 0 0 1"
got=""
for _ in $(seq 1 20); do
  got="$(query <<'SQL'
WITH mine AS (
  SELECT p.id, p.cabal_id, p.kind, p.usdc_micros, p.status
  FROM proposals p JOIN cabals c ON c.id = p.cabal_id JOIN users u ON u.id = p.proposer_id
  WHERE c.name = :'name' AND u.privy_user_id = :'a' AND p.kind = 'buy' AND p.symbol LIKE 'AAPL%'
), buy AS (
  SELECT s.* FROM swaps s WHERE s.source_kind = 'proposal' AND s.source_id IN (SELECT id FROM mine) AND s.action = 'buy'
)
SELECT count(*), max(kind), max(usdc_micros), max(status),
  (SELECT count(*) FROM buy),
  coalesce((SELECT max(status) FROM buy), 'none'),
  (SELECT count(*) FROM trade_routes r WHERE r.trade_id IN (SELECT id FROM buy)),
  coalesce((SELECT jsonb_array_length(r.comparison -> 'ranked') >= 2 FROM trade_routes r WHERE r.trade_id IN (SELECT id FROM buy)), false),
  coalesce((SELECT r.comparison -> 'ranked' @> jsonb_build_array(jsonb_build_object('listing_id', r.winner_listing_id::text))
    FROM trade_routes r WHERE r.trade_id IN (SELECT id FROM buy)), false),
  (SELECT count(*) FROM trade_steps t WHERE t.trade_id IN (SELECT id FROM buy) AND t.status <> 'succeeded'),
  (SELECT coalesce(sum(cp.units), 0) FROM cabal_positions cp JOIN buy s ON s.cabal_id = cp.cabal_id AND s.out_mint = cp.asset),
  (SELECT count(*) FROM user_txns t JOIN users u ON u.id = t.user_id
    WHERE t.cabal_id IN (SELECT cabal_id FROM mine) AND t.kind = 'cash_out' AND t.status = 'settled' AND u.privy_user_id = :'a')
FROM mine
SQL
)"
  [[ "$got" == "$want" ]] && break
  sleep 1
done
if [[ "$got" != "$want" ]]; then
  echo "A's Apple proposal in $name: got '$got', want '$want' (count kind usdc_micros status buy-swaps swap-status routes two-or-more-quotes winner-quoted unfinished-steps cabal-units-left settled-cash-outs)"
  exit 1
fi
echo "ok: $name has A's executed buy of Apple, one confirmed swap routed to the best of at least two quotes with every step done, nothing left after the refund and a settled cash out"
