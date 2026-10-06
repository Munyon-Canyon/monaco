-- name: LiveAgentOfCabal :one
SELECT id, cabal_id, name, status, budget_usdc_micros FROM agents
WHERE cabal_id = sqlc.arg(cabal_id) AND status <> 'removed';
