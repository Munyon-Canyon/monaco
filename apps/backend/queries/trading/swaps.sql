-- name: InsertCreated :exec
INSERT INTO swaps (
  id, source_kind, source_id, cabal_id, treasury_address, action, symbol,
  in_mint, out_mint, out_decimals, in_amount, quote_out_amount, slippage_bps, source_batch_size, status, created_at, updated_at
)
VALUES (
  @id, @source_kind, @source_id, @cabal_id, @treasury_address, @action, @symbol,
  @in_mint, @out_mint, @out_decimals, @in_amount, @quote_out_amount, @slippage_bps, @source_batch_size, 'created', @created_at, @created_at
);

-- name: MarkSubmitted :execrows
UPDATE swaps
SET status = 'submitted', execute_request_id = @execute_request_id::text, signed_tx = @signed_tx::bytea,
  tx_signature = @tx_signature::text, submitted_at = @submitted_at::timestamptz, updated_at = @submitted_at::timestamptz
WHERE id = @id AND status = 'created' AND octet_length(@signed_tx::bytea) > 0;

-- name: FinishConfirmed :execrows
UPDATE swaps
SET status = 'confirmed', out_amount = @out_amount::bigint, fee_micros = @fee_micros::bigint,
  confirmed_at = @confirmed_at::timestamptz, updated_at = @confirmed_at::timestamptz
WHERE id = @id AND status = 'submitted';

-- name: FinishFailed :execrows
UPDATE swaps
SET status = 'failed', failure_code = @failure_code::text,
  failed_at = @failed_at::timestamptz, updated_at = @failed_at::timestamptz
WHERE id = @id
  AND CASE status
    WHEN 'created' THEN @failure_code::text IN ('never_submitted', 'source_cancelled', 'price_moved')
    WHEN 'submitted' THEN @failure_code::text IN ('blockhash_expired', 'jupiter_failed', 'force_resolved', 'swap_reverted')
    ELSE false
  END;

-- name: SwapForForceResolve :one
SELECT id, cabal_id, source_kind, source_id, action, symbol, in_mint, in_amount, out_mint, out_decimals,
  source_batch_size, status, tx_signature, treasury_address, signed_tx,
  EXISTS (SELECT 1 FROM trade_steps WHERE trade_steps.trade_id = swaps.id) AS routed,
  coalesce(parked_code = 'swap_receipt_missing', false)::bool AS receipt_missing
FROM swaps
WHERE tx_signature = @tx_signature::text;

-- name: ClaimLive :one
SELECT id, status
FROM swaps
WHERE source_kind = @source_kind AND source_id = @source_id AND in_mint = @in_mint AND status <> 'failed';

-- name: ListStaleCreated :many
SELECT id, cabal_id, source_kind, source_id, action, symbol, in_mint, source_batch_size, in_amount
FROM swaps
WHERE swaps.status = 'created' AND swaps.created_at < @older_than
  AND NOT EXISTS (SELECT 1 FROM trade_steps WHERE trade_steps.trade_id = swaps.id)
ORDER BY swaps.updated_at, swaps.id
LIMIT @max_rows;

-- name: ListStaleSubmitted :many
SELECT id, cabal_id, source_kind, source_id, action, symbol, in_mint, out_mint, out_decimals, in_amount,
  source_batch_size, treasury_address, tx_signature, signed_tx
FROM swaps
WHERE swaps.status = 'submitted' AND swaps.submitted_at < @older_than::timestamptz
  AND NOT EXISTS (SELECT 1 FROM trade_steps WHERE trade_steps.trade_id = swaps.id)
ORDER BY swaps.updated_at, swaps.id
LIMIT @max_rows;

-- name: TouchSubmitted :execrows
UPDATE swaps
SET updated_at = @updated_at
WHERE id = @id AND status = 'submitted';

-- name: TouchCreated :execrows
UPDATE swaps
SET updated_at = @updated_at
WHERE id = @id AND status = 'created';
