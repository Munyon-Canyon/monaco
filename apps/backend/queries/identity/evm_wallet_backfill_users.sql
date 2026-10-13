-- name: EVMWalletBackfillUsers :many
SELECT w.user_id, (w.evm_privy_wallet_id IS NOT NULL)::boolean AS has_evm_wallet
FROM user_wallets w
JOIN users u ON u.id = w.user_id
WHERE u.deleted_at IS NULL
ORDER BY w.user_id;
