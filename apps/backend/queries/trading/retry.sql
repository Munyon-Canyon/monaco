-- name: SwapForRetry :one
SELECT s.id, s.cabal_id, s.source_kind, s.source_id, s.action, s.symbol, s.in_mint, s.out_mint, s.in_amount,
  s.quote_out_amount, s.slippage_bps, v.retryable::bool AS retryable
FROM swaps s
JOIN swap_views v ON v.id = s.id
WHERE s.id = @id;
