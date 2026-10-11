-- name: CabalValueRows :many
WITH head AS (
  SELECT
    (SELECT coalesce(sum(payout_micros), 0)
     FROM cash_out_jobs
     WHERE cabal_id = sqlc.arg(cabal_id)::uuid
       AND status IN ('started', 'selling', 'paying'))::text AS cash_out_reserved_micros,
    (SELECT coalesce(sum(amount_micros), 0)
     FROM treasury_in_transit
     WHERE cabal_id = sqlc.arg(cabal_id)::uuid)::text AS in_transit_micros
)
SELECT head.cash_out_reserved_micros, head.in_transit_micros, p.asset,
  coalesce(p.units, 0)::text AS units, coalesce(p.cost_basis_micros, 0)::text AS cost_basis_micros
FROM head
LEFT JOIN cabal_positions AS p ON p.cabal_id = sqlc.arg(cabal_id)::uuid AND p.units > 0
ORDER BY p.asset;

-- name: ListActivityRouted :many
SELECT a.id, a.kind,
  CASE
    WHEN a.status = 'confirmed' AND a.kind IN ('buy', 'sell') AND EXISTS (
      SELECT 1 FROM trade_steps AS s WHERE s.trade_id = a.id AND s.status IN ('pending', 'submitted')
    ) THEN 'pending'
    ELSE a.status
  END::text AS status,
  a.actor_user_id, a.asset, a.usdc_micros, a.units, a.tx_signature, a.occurred_at
FROM cabal_activity AS a
WHERE a.cabal_id = @cabal_id
  AND (NOT @has_cursor::bool OR (a.occurred_at, a.id) < (@cursor_at::timestamptz, @cursor_id::uuid))
ORDER BY a.occurred_at DESC, a.id DESC
LIMIT @row_limit;
