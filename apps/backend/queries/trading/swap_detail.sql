-- name: SwapDetail :one
SELECT v.id, v.cabal_id, v.source_kind, v.source_id, v.action, v.symbol, v.in_amount, v.out_amount, v.status,
  v.failure_code, v.tx_signature, v.created_at, v.confirmed_at, (v.retryable AND v.source_kind = 'proposal')::bool AS retryable,
  (CASE WHEN s.action = 'buy' THEN s.out_mint ELSE s.in_mint END)::text AS token_mint
FROM swap_views v
JOIN swaps s ON s.id = v.id
WHERE v.id = @id;
