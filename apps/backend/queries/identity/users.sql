-- name: InsertUser :exec
INSERT INTO users (id, privy_user_id, login_provider, email, auth_state_changed_at, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(privy_user_id), sqlc.arg(login_provider), sqlc.narg(email),
  sqlc.arg(now), sqlc.arg(now), sqlc.arg(now));

-- name: InsertUserWallet :exec
INSERT INTO user_wallets (user_id, privy_wallet_id, address, created_at)
VALUES ($1, $2, $3, $4);

-- name: FindUserByPrivyUserID :one
SELECT u.id, u.privy_user_id, u.handle, u.auth_state, u.account_status, w.privy_wallet_id, w.address
FROM users u
LEFT JOIN user_wallets w ON w.user_id = u.id
WHERE u.privy_user_id = $1;

-- name: FindUserByID :one
SELECT u.id, u.privy_user_id, u.handle, u.auth_state, u.account_status, w.privy_wallet_id, w.address
FROM users u
LEFT JOIN user_wallets w ON w.user_id = u.id
WHERE u.id = $1 AND u.deleted_at IS NULL;

-- name: UpdateAuthState :execrows
UPDATE users SET auth_state = sqlc.arg(next), auth_state_changed_at = sqlc.arg(now), updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND auth_state = sqlc.arg(expected);

-- name: UpdateAccountStatus :execrows
UPDATE users SET account_status = sqlc.arg(next), updated_at = sqlc.arg(now),
  deleted_at = CASE WHEN sqlc.arg(next)::text = 'deleted' THEN sqlc.arg(now)::timestamptz ELSE deleted_at END
WHERE id = sqlc.arg(id) AND account_status = sqlc.arg(expected);
