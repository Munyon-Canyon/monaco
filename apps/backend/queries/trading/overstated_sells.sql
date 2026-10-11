-- name: ListConfirmedSellsInto :many
SELECT id, cabal_id, symbol, treasury_address, out_amount, tx_signature
FROM swaps
WHERE action = 'sell' AND status = 'confirmed' AND out_mint = @out_mint::text AND tx_signature IS NOT NULL
ORDER BY cabal_id, confirmed_at, id;
