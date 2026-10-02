-- name: InsertCreated :exec
INSERT INTO swaps (
  id, source_kind, source_id, cabal_id, treasury_address, action, symbol,
  in_mint, out_mint, in_amount, quote_out_amount, slippage_bps, status, created_at, updated_at
)
VALUES (
  @id, @source_kind, @source_id, @cabal_id, @treasury_address, @action, @symbol,
  @in_mint, @out_mint, @in_amount, @quote_out_amount, @slippage_bps, 'created', @created_at, @created_at
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
    WHEN 'created' THEN @failure_code::text = 'never_submitted'
    WHEN 'submitted' THEN @failure_code::text <> 'never_submitted'
    ELSE false
  END;

-- name: ClaimLive :one
SELECT id, status
FROM swaps
WHERE source_kind = @source_kind AND source_id = @source_id AND in_mint = @in_mint AND status <> 'failed';
