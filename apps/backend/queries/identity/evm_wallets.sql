-- name: SetUserEVMWallet :execrows
UPDATE user_wallets
SET evm_privy_wallet_id = sqlc.arg(evm_privy_wallet_id), evm_address = sqlc.arg(evm_address),
  evm_created_at = sqlc.arg(evm_created_at)
WHERE user_id = sqlc.arg(user_id) AND evm_privy_wallet_id IS NULL;

-- name: FindUserEVMWallet :one
SELECT user_id, evm_privy_wallet_id::text AS evm_privy_wallet_id, evm_address::text AS evm_address,
  evm_created_at::timestamptz AS evm_created_at
FROM user_wallets
WHERE user_id = $1 AND evm_privy_wallet_id IS NOT NULL;

-- name: EVMDepositWalletsAfter :many
SELECT user_id, evm_privy_wallet_id::text AS evm_privy_wallet_id
FROM user_wallets
WHERE evm_privy_wallet_id IS NOT NULL AND user_id > sqlc.arg(after)
ORDER BY user_id
LIMIT sqlc.arg(page_size)::bigint;
