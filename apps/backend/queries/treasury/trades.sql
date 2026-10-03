-- name: SwapPosted :one
SELECT EXISTS (SELECT 1 FROM cabal_txns WHERE swap_id = sqlc.arg(swap_id)::uuid);
