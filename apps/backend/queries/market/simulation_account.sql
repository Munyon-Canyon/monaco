-- name: SimulationAccount :one
SELECT privy_wallet_id, address FROM evm_simulation_account WHERE singleton;

-- name: InsertSimulationAccount :execrows
INSERT INTO evm_simulation_account (privy_wallet_id, address, created_at)
VALUES (sqlc.arg(privy_wallet_id), sqlc.arg(address), sqlc.arg(created_at))
ON CONFLICT (singleton) DO NOTHING;
