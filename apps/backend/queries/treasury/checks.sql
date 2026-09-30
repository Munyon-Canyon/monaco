-- name: UnbalancedTxns :many
SELECT 'cabal_txns'::text AS ledger, txn_id::text AS txn_id, asset, sum(amount)::text AS total
FROM cabal_txn_entries GROUP BY txn_id, asset HAVING sum(amount) <> 0
UNION ALL
SELECT 'user_txns'::text, txn_id::text, asset, sum(amount)::text
FROM user_txn_entries GROUP BY txn_id, asset HAVING sum(amount) <> 0
ORDER BY 1, 2, 3;

-- name: SplitTransfers :many
SELECT s.transfer_id::text AS transfer_id, string_agg(DISTINCT s.status, ',' ORDER BY s.status)::text AS statuses
FROM (
  SELECT transfer_id, status FROM cabal_txns WHERE transfer_id IS NOT NULL
  UNION ALL
  SELECT transfer_id, status FROM user_txns WHERE transfer_id IS NOT NULL
) s
GROUP BY s.transfer_id HAVING count(DISTINCT s.status) > 1
ORDER BY 1;

-- name: CabalPositionDrift :many
WITH sums AS (
  SELECT t.cabal_id, e.asset, sum(e.amount) AS units
  FROM cabal_txn_entries e JOIN cabal_txns t ON t.id = e.txn_id
  WHERE e.account = 'treasury'
  GROUP BY t.cabal_id, e.asset
)
SELECT coalesce(s.cabal_id, p.cabal_id)::text AS cabal_id, coalesce(s.asset, p.asset)::text AS asset,
  coalesce(s.units, 0)::text AS entries, coalesce(p.units, 0)::text AS position
FROM sums s FULL JOIN cabal_positions p ON p.cabal_id = s.cabal_id AND p.asset = s.asset
WHERE coalesce(s.units, 0) <> coalesce(p.units, 0)
ORDER BY 1, 2;

-- name: UserPositionDrift :many
WITH sums AS (
  SELECT t.user_id, t.cabal_id,
    coalesce(sum(e.amount) FILTER (WHERE e.account = 'holder' AND e.asset = 'shares:' || t.cabal_id::text), 0)
      AS shares,
    coalesce(sum(e.amount) FILTER (WHERE e.account = 'cabal' AND e.amount > 0), 0)
      AS contributed,
    coalesce(-sum(e.amount) FILTER (WHERE e.account = 'cabal' AND e.amount < 0), 0)
      AS withdrawn
  FROM user_txn_entries e JOIN user_txns t ON t.id = e.txn_id
  WHERE t.cabal_id IS NOT NULL
  GROUP BY t.user_id, t.cabal_id
)
SELECT coalesce(s.user_id, p.user_id)::text AS user_id, coalesce(s.cabal_id, p.cabal_id)::text AS cabal_id,
  concat_ws('/', coalesce(s.shares, 0), coalesce(s.contributed, 0), coalesce(s.withdrawn, 0))::text AS entries,
  concat_ws('/', coalesce(p.share_units, 0), coalesce(p.contributed_micros, 0), coalesce(p.withdrawn_micros, 0))::text
    AS position
FROM sums s FULL JOIN user_positions p ON p.user_id = s.user_id AND p.cabal_id = s.cabal_id
WHERE (coalesce(s.shares, 0), coalesce(s.contributed, 0), coalesce(s.withdrawn, 0))
  IS DISTINCT FROM (coalesce(p.share_units, 0), coalesce(p.contributed_micros, 0), coalesce(p.withdrawn_micros, 0))
ORDER BY 1, 2;
