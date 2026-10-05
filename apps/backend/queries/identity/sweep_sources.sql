-- name: ListSweepMemberWallets :many
SELECT privy_wallet_id, address FROM user_wallets
ORDER BY created_at, user_id;
