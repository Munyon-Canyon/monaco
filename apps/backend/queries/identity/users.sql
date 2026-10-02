-- name: CreateUser :execrows
INSERT INTO users (id, privy_user_id, login_provider, auth_state_changed_at, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(privy_user_id), sqlc.arg(login_provider), sqlc.arg(now), sqlc.arg(now), sqlc.arg(now))
ON CONFLICT (privy_user_id) DO NOTHING;

-- name: AttachUserWallet :execrows
INSERT INTO user_wallets (user_id, privy_wallet_id, address, created_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING;

-- name: SetUserEmail :exec
UPDATE users SET email = sqlc.narg(email), updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND email IS DISTINCT FROM sqlc.narg(email);

-- name: FindUserByPrivyUserID :one
SELECT u.id, u.privy_user_id, u.handle, u.auth_state, u.account_status, u.phone_e164, u.x_user_id, u.x_username,
  w.privy_wallet_id, w.address
FROM users u
LEFT JOIN user_wallets w ON w.user_id = u.id
WHERE u.privy_user_id = $1;

-- name: LockUserByPrivyUserID :one
SELECT u.id, u.privy_user_id, u.handle, u.auth_state, u.account_status, u.phone_e164, u.x_user_id, u.x_username,
  w.privy_wallet_id, w.address
FROM users u
LEFT JOIN user_wallets w ON w.user_id = u.id
WHERE u.privy_user_id = $1
FOR UPDATE OF u;

-- name: FindUserByID :one
SELECT u.id, u.privy_user_id, u.handle, u.auth_state, u.account_status, u.phone_e164, u.x_user_id, u.x_username,
  w.privy_wallet_id, w.address
FROM users u
LEFT JOIN user_wallets w ON w.user_id = u.id
WHERE u.id = $1 AND u.deleted_at IS NULL;

-- name: SetUserPhone :exec
UPDATE users SET phone_e164 = sqlc.narg(phone_e164), phone_hash = sqlc.narg(phone_hash),
  phone_verified_at = sqlc.narg(verified_at), updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id);

-- name: SetUserX :exec
UPDATE users SET x_user_id = sqlc.narg(x_user_id), x_username = sqlc.narg(x_username),
  x_linked_at = sqlc.narg(linked_at), updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id);

-- name: LinksHeldByOthers :one
SELECT
  EXISTS (
    SELECT 1 FROM users o
    WHERE sqlc.narg(phone_hash)::bytea IS NOT NULL
      AND o.phone_hash = sqlc.narg(phone_hash)::bytea
      AND o.id <> sqlc.arg(id)
  ) AS phone,
  EXISTS (
    SELECT 1 FROM users o
    WHERE sqlc.narg(x_user_id)::text IS NOT NULL
      AND o.x_user_id = sqlc.narg(x_user_id)::text
      AND o.id <> sqlc.arg(id)
  ) AS x;

-- name: UpdateAuthState :execrows
UPDATE users SET auth_state = sqlc.arg(next), auth_state_changed_at = sqlc.arg(now), updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND auth_state = sqlc.arg(expected);

-- name: UpdateAccountStatus :execrows
UPDATE users SET account_status = sqlc.arg(next), updated_at = sqlc.arg(now),
  deleted_at = CASE WHEN sqlc.arg(next)::text = 'deleted' THEN sqlc.arg(now)::timestamptz ELSE deleted_at END
WHERE id = sqlc.arg(id) AND account_status = sqlc.arg(expected);

-- name: InsertDevUser :exec
INSERT INTO users (
  id, privy_user_id, login_provider, handle, display_name, auth_state_changed_at, created_at, updated_at
) VALUES (
  sqlc.arg(id), sqlc.arg(privy_user_id), 'email', sqlc.arg(handle), sqlc.arg(display_name),
  sqlc.arg(now), sqlc.arg(now), sqlc.arg(now)
);
