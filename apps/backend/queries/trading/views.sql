-- name: SwapByID :one
SELECT id, cabal_id, source_kind, source_id, action, symbol, in_amount, out_decimals, out_amount, status, failure_code, tx_signature, created_at, confirmed_at, retryable FROM swap_views WHERE id = @id;

-- name: SwapBySignature :one
SELECT id, cabal_id, source_kind, source_id, action, symbol, in_amount, out_decimals, out_amount, status, failure_code, tx_signature, created_at, confirmed_at, retryable FROM swap_views WHERE tx_signature = @tx_signature::text;

-- name: LatestBySource :one
SELECT id, cabal_id, source_kind, source_id, action, symbol, in_amount, out_decimals, out_amount, status, failure_code, tx_signature, created_at, confirmed_at, retryable FROM swap_views
WHERE source_kind = @source_kind AND source_id = @source_id
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: HasLiveSwap :one
SELECT EXISTS (
  SELECT 1 FROM swaps WHERE source_kind = @source_kind AND source_id = @source_id AND status <> 'failed'
);

-- name: OwnsSignature :one
SELECT EXISTS (SELECT 1 FROM swaps WHERE tx_signature = @tx_signature::text);
