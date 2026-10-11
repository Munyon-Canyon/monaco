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
  AND s.parked_at IS NULL
  AND EXISTS (SELECT 1 FROM trade_steps AS t WHERE t.trade_id = s.id)
ORDER BY s.updated_at, s.id
LIMIT sqlc.arg(max_rows);

-- name: TouchRouted :execrows
UPDATE swaps
SET updated_at = sqlc.arg(at)::timestamptz
WHERE id = sqlc.arg(id)::uuid AND status IN ('created', 'submitted');

-- name: LockStrandedCash :exec
SELECT pg_advisory_xact_lock(hashtextextended('stranded-cash:' || sqlc.arg(swap_id)::uuid::text, 0));

-- name: StrandedCash :one
SELECT EXISTS (
  SELECT 1
  FROM trade_steps AS o
  JOIN swaps AS s ON s.id = o.trade_id
  WHERE o.trade_id = sqlc.arg(swap_id)::uuid AND o.step = 'bridge_out' AND o.status = 'succeeded'
    AND (s.status = 'failed' OR s.parked_at IS NOT NULL)
    AND NOT EXISTS (
      SELECT 1 FROM trade_steps AS t
      WHERE t.trade_id = o.trade_id AND (t.step = 'bridge_back' OR (t.step = 'swap' AND t.status <> 'failed'))
    )
    AND NOT EXISTS (SELECT 1 FROM trade_steps AS r WHERE r.privy_action_id = 'reused:' || o.trade_id::text)
)::bool AS stranded;

-- name: ReuseStrandedCash :execrows
INSERT INTO trade_steps (id, trade_id, step, network, privy_action_id, amount_micros, status, created_at, updated_at)
SELECT sqlc.arg(id)::uuid, sqlc.arg(to_trade)::uuid, 'bridge_out', o.network, 'reused:' || o.trade_id::text,
  o.amount_micros, 'succeeded', sqlc.arg(at)::timestamptz, sqlc.arg(at)::timestamptz
FROM trade_steps AS o
JOIN swaps AS s ON s.id = o.trade_id
WHERE o.trade_id = sqlc.arg(from_trade)::uuid AND o.step = 'bridge_out' AND o.status = 'succeeded'
  AND s.status = 'failed'
  AND NOT EXISTS (
    SELECT 1 FROM trade_steps AS t
    WHERE t.trade_id = o.trade_id AND (t.step = 'bridge_back' OR (t.step = 'swap' AND t.status <> 'failed'))
  )
  AND NOT EXISTS (SELECT 1 FROM trade_steps AS r WHERE r.privy_action_id = 'reused:' || o.trade_id::text);

-- name: ListStrandedCash :many
SELECT s.id, s.source_kind, s.source_id,
  EXISTS (SELECT 1 FROM trade_steps AS b WHERE b.trade_id = s.id AND b.step = 'bridge_back')::bool AS returning
FROM swaps AS s
WHERE coalesce(s.failed_at, s.confirmed_at, s.parked_at) < sqlc.arg(older_than)::timestamptz
  AND (
    EXISTS (
      SELECT 1 FROM trade_steps AS b
      WHERE b.trade_id = s.id AND b.step = 'bridge_back' AND b.status IN ('pending', 'submitted')
    )
    OR (
      (s.status = 'failed' OR s.parked_at IS NOT NULL)
      AND EXISTS (
        SELECT 1 FROM trade_steps AS o WHERE o.trade_id = s.id AND o.step = 'bridge_out' AND o.status = 'succeeded'
      )
      AND NOT EXISTS (
        SELECT 1 FROM trade_steps AS t
        WHERE t.trade_id = s.id AND (t.step = 'bridge_back' OR (t.step = 'swap' AND t.status <> 'failed'))
      )
      AND NOT EXISTS (SELECT 1 FROM trade_steps AS r WHERE r.privy_action_id = 'reused:' || s.id::text)
    )
  )
ORDER BY s.updated_at, s.id
LIMIT sqlc.arg(max_rows);

-- name: TouchFinished :execrows
UPDATE swaps
SET updated_at = sqlc.arg(at)::timestamptz
WHERE id = sqlc.arg(id)::uuid AND status IN ('failed', 'confirmed');

-- name: ParkRouted :one
UPDATE swaps AS s
SET parked_at = sqlc.arg(at)::timestamptz, parked_code = sqlc.arg(code)::text
WHERE s.id = sqlc.arg(id)::uuid AND s.parked_at IS NULL AND s.status IN ('created', 'submitted')
RETURNING s.cabal_id, coalesce((
  SELECT t.step FROM trade_steps AS t WHERE t.trade_id = s.id ORDER BY t.updated_at DESC, t.created_at DESC LIMIT 1
), '')::text AS step;
