-- name: SwapByID :one
SELECT id, cabal_id, source_kind, source_id, action, symbol, in_amount, out_decimals, out_amount, status, failure_code, tx_signature, created_at, confirmed_at, retryable, fee_micros FROM swap_views WHERE id = @id;

-- name: SwapBySignature :one
SELECT id, cabal_id, source_kind, source_id, action, symbol, in_amount, out_decimals, out_amount, status, failure_code, tx_signature, created_at, confirmed_at, retryable, fee_micros FROM swap_views WHERE tx_signature = @tx_signature::text;

-- name: LatestBySource :one
SELECT id, cabal_id, source_kind, source_id, action, symbol, in_amount, out_decimals, out_amount, status, failure_code, tx_signature, created_at, confirmed_at, retryable, fee_micros FROM swap_views
WHERE source_kind = @source_kind AND source_id = @source_id
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: HasLiveSwap :one
SELECT EXISTS (
  SELECT 1 FROM swaps WHERE source_kind = @source_kind AND source_id = @source_id AND status <> 'failed'
);

-- name: OwnsSignature :one
SELECT EXISTS (
  SELECT 1 FROM swaps WHERE tx_signature = @tx_signature::text
  UNION ALL
  SELECT 1 FROM trade_steps WHERE step = 'bridge_back' AND arrival_tx = @tx_signature::text
);

-- name: BridgeBackInFlight :one
SELECT coalesce(sum(t.amount_micros), 0)::text AS in_flight_micros
FROM trade_steps t
JOIN swaps s ON s.id = t.trade_id
WHERE s.cabal_id = @cabal_id::uuid AND t.step = 'bridge_back' AND t.status = 'submitted';
