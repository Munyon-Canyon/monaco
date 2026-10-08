-- name: UserCardsByID :many
SELECT id, handle, display_name, photo_url, auth_state, account_status,
  (phone_verified_at IS NOT NULL)::boolean AS phone_verified,
  (x_user_id IS NOT NULL)::boolean AS x_linked,
  auth_state_changed_at, created_at, first_deposit_at,
  (deleted_at IS NOT NULL)::boolean AS deleted
FROM users
WHERE id = ANY(sqlc.arg(user_ids)::uuid[]);

-- name: UserCardByHandle :one
SELECT id, handle, display_name, photo_url, auth_state, account_status,
  (phone_verified_at IS NOT NULL)::boolean AS phone_verified,
  (x_user_id IS NOT NULL)::boolean AS x_linked,
  auth_state_changed_at, created_at, first_deposit_at,
  (deleted_at IS NOT NULL)::boolean AS deleted
FROM users
WHERE handle = sqlc.arg(handle)::text AND deleted_at IS NULL;

-- name: UserIDsByHandles :many
SELECT id, handle
FROM users
WHERE handle = ANY(sqlc.arg(handles)::text[]) AND deleted_at IS NULL;

-- name: UserIDsByPhoneHashes :many
SELECT id, phone_hash
FROM users
WHERE phone_hash = ANY(sqlc.arg(hashes)::bytea[])
  AND phone_verified_at IS NOT NULL AND account_status = 'active' AND deleted_at IS NULL;

-- name: UserIDsByXUserIDs :many
SELECT id, x_user_id
FROM users
WHERE x_user_id = ANY(sqlc.arg(x_user_ids)::text[]) AND account_status = 'active' AND deleted_at IS NULL;

-- name: MemberWalletByUserID :one
SELECT w.privy_wallet_id, w.address
FROM user_wallets w
JOIN users u ON u.id = w.user_id
WHERE w.user_id = sqlc.arg(user_id)::uuid AND u.deleted_at IS NULL;

-- name: MemberWalletsAfter :many
SELECT w.user_id, w.privy_wallet_id, w.address
FROM user_wallets w
JOIN users u ON u.id = w.user_id
WHERE w.user_id > sqlc.arg(after)::uuid AND u.deleted_at IS NULL
ORDER BY w.user_id
LIMIT sqlc.arg(page_size)::bigint;

-- name: UserStatusCounts :many
SELECT auth_state, account_status, count(*)::bigint AS users
FROM users
GROUP BY auth_state, account_status
ORDER BY auth_state, account_status;
