-- name: UnbookedSwaps :many
SELECT s.id, s.cabal_id, s.confirmed_at::timestamptz AS since
FROM swaps AS s
WHERE s.status = 'confirmed'
  AND s.confirmed_at > sqlc.arg(after)::timestamptz
  AND s.confirmed_at < sqlc.arg(before)::timestamptz
  AND NOT EXISTS (SELECT 1 FROM cabal_txns AS c WHERE c.swap_id = s.id)
ORDER BY s.confirmed_at, s.id
LIMIT sqlc.arg(max_rows)::int;

-- name: SettledBridgeSteps :many
SELECT t.trade_id, t.step, t.network, t.status, t.attempt, coalesce(t.privy_action_id, '')::text AS privy_action_id,
  s.cabal_id, t.updated_at
FROM trade_steps AS t
JOIN swaps AS s ON s.id = t.trade_id
WHERE t.step IN ('bridge_out', 'bridge_back')
  AND t.status IN ('succeeded', 'failed')
  AND t.updated_at > sqlc.arg(after)::timestamptz
  AND t.updated_at < sqlc.arg(before)::timestamptz
ORDER BY t.updated_at, t.trade_id, t.step;

-- name: BookedCabalTxns :many
SELECT id FROM cabal_txns WHERE id = ANY(sqlc.arg(ids)::uuid[]);
