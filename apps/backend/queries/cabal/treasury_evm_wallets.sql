-- name: InsertTreasuryEVMWallet :exec
INSERT INTO treasury_evm_wallets (cabal_id, privy_wallet_id, address, created_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING;

-- name: FindTreasuryEVMWallet :one
SELECT cabal_id, privy_wallet_id, address, created_at FROM treasury_evm_wallets
WHERE cabal_id = $1;
