-- name: InsertTradeStep :execrows
INSERT INTO trade_steps (id, trade_id, step, network, amount_micros, status, created_at, updated_at)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(trade_id)::uuid, sqlc.arg(step)::text, sqlc.arg(network)::text,
  sqlc.arg(amount_micros)::text::numeric, 'pending', sqlc.arg(at)::timestamptz, sqlc.arg(at)::timestamptz)
ON CONFLICT (trade_id, step) DO NOTHING;

-- name: MarkTradeStepSubmitted :execrows
UPDATE trade_steps
SET privy_action_id = sqlc.arg(privy_action_id)::text, status = 'submitted', updated_at = sqlc.arg(at)::timestamptz
WHERE trade_id = sqlc.arg(trade_id)::uuid AND step = sqlc.arg(step)::text AND status = 'pending';

-- name: FinishTradeStep :execrows
UPDATE trade_steps
SET status = sqlc.arg(status)::text, error = NULLIF(sqlc.arg(error)::text, ''), updated_at = sqlc.arg(at)::timestamptz
WHERE trade_id = sqlc.arg(trade_id)::uuid AND step = sqlc.arg(step)::text AND status IN ('pending', 'submitted');

-- name: TradeStep :one
SELECT t.id, t.trade_id, t.step, t.network, coalesce(t.privy_action_id, '')::text AS privy_action_id,
  t.amount_micros::text AS amount_micros, t.status, coalesce(t.error, '')::text AS error, t.created_at, t.updated_at,
  s.source_kind, s.source_id, s.source_batch_size
FROM trade_steps AS t
JOIN swaps AS s ON s.id = t.trade_id
WHERE t.trade_id = sqlc.arg(trade_id)::uuid AND t.step = sqlc.arg(step)::text;
