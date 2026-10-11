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
SELECT id, trade_id, step, network, coalesce(privy_action_id, '')::text AS privy_action_id,
  amount_micros::text AS amount_micros, status, coalesce(error, '')::text AS error, created_at, updated_at
FROM trade_steps
WHERE trade_id = sqlc.arg(trade_id)::uuid AND step = sqlc.arg(step)::text;
