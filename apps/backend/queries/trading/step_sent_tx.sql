-- name: RecordTradeStepSentTx :execrows
UPDATE trade_steps
SET sent_tx = sqlc.arg(sent_tx)::jsonb
WHERE trade_id = sqlc.arg(trade_id)::uuid AND step = sqlc.arg(step)::text AND status = 'pending'
  AND sent_tx IS NULL;

-- name: TradeStepSentTx :one
SELECT sent_tx
FROM trade_steps
WHERE trade_id = sqlc.arg(trade_id)::uuid AND step = sqlc.arg(step)::text;
