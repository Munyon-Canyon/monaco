-- name: MarkSubmittedEVM :execrows
UPDATE swaps
SET status = 'submitted', tx_signature = sqlc.arg(tx_hash)::text, submitted_at = sqlc.arg(submitted_at)::timestamptz,
  updated_at = sqlc.arg(submitted_at)::timestamptz
WHERE id = sqlc.arg(id)::uuid AND status = 'created' AND sqlc.arg(tx_hash)::text <> '';

-- name: RoutedSwap :one
SELECT s.id, s.cabal_id, s.source_kind, s.source_id, s.action, s.symbol, s.in_mint, s.out_mint, s.in_amount,
  s.slippage_bps, s.source_batch_size, s.status, r.comparison
FROM swaps AS s
JOIN trade_routes AS r ON r.trade_id = s.id
WHERE s.id = sqlc.arg(id)::uuid;

-- name: ListRoutedUnfinished :many
SELECT s.id
FROM swaps AS s
WHERE s.status IN ('created', 'submitted') AND s.created_at < sqlc.arg(older_than)::timestamptz
  AND EXISTS (SELECT 1 FROM trade_steps AS t WHERE t.trade_id = s.id)
ORDER BY s.updated_at, s.id
LIMIT sqlc.arg(max_rows);

-- name: TouchRouted :execrows
UPDATE swaps
SET updated_at = sqlc.arg(at)::timestamptz
WHERE id = sqlc.arg(id)::uuid AND status IN ('created', 'submitted');
