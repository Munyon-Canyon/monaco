-- name: ListActivity :many
SELECT id, kind, status, actor_user_id, asset, usdc_micros, units,
  tx_signature, occurred_at
FROM cabal_activity
WHERE cabal_id = @cabal_id AND NOT cashout_sale
  AND (NOT @has_cursor::bool OR (occurred_at, id) < (@cursor_at::timestamptz, @cursor_id::uuid))
ORDER BY occurred_at DESC, id DESC
LIMIT @row_limit;

-- name: ListTradesForPairs :many
SELECT a.id, a.cabal_id, a.kind, a.asset::text AS asset, a.usdc_micros, a.units, a.occurred_at,
  a.cashout_sale
FROM cabal_activity AS a
JOIN (
  SELECT c.cabal_id, k.asset
  FROM unnest(sqlc.arg(cabal_ids)::uuid[]) WITH ORDINALITY AS c(cabal_id, idx)
  JOIN unnest(sqlc.arg(assets)::text[]) WITH ORDINALITY AS k(asset, idx) USING (idx)
) AS p ON p.cabal_id = a.cabal_id AND p.asset = a.asset
WHERE a.kind IN ('buy', 'sell') AND a.status = 'confirmed'
  AND a.usdc_micros IS NOT NULL AND a.units IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM trade_steps AS s WHERE s.trade_id = a.id AND s.status IN ('pending', 'submitted'))
ORDER BY a.cabal_id, a.asset, a.occurred_at, a.id;
