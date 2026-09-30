-- name: InsertTreasuryWallet :exec
INSERT INTO treasury_wallets (cabal_id, privy_wallet_id, address, created_at)
VALUES ($1, $2, $3, $4);

-- name: FindTreasuryWallet :one
SELECT cabal_id, privy_wallet_id, address, created_at FROM treasury_wallets
WHERE cabal_id = $1;

-- name: ListTreasuryWallets :many
SELECT cabal_id, privy_wallet_id, address, created_at FROM treasury_wallets
ORDER BY cabal_id;
