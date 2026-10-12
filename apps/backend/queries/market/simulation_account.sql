-- name: SimulationAccount :one
SELECT coalesce(privy_wallet_id, '')::text AS privy_wallet_id, address,
  coalesce(legacy_network, '')::text AS legacy_network
FROM evm_simulation_account WHERE singleton;

-- name: ReplaceSimulationAccount :execrows
INSERT INTO evm_simulation_account (privy_wallet_id, address, created_at, legacy_network)
VALUES (sqlc.arg(privy_wallet_id)::text, sqlc.arg(address), sqlc.arg(created_at), NULL)
ON CONFLICT (singleton) DO UPDATE SET
  privy_wallet_id = EXCLUDED.privy_wallet_id,
  address = EXCLUDED.address,
  created_at = EXCLUDED.created_at,
  legacy_network = NULL
WHERE evm_simulation_account.legacy_network IS NOT NULL;
