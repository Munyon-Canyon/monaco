-- name: GetMe :one
SELECT u.id, u.handle, u.handle_changed_at, u.display_name, u.photo_url, u.auth_state, u.account_status,
  (u.phone_verified_at IS NOT NULL)::boolean AS phone_linked, u.x_username, u.created_at, w.address
FROM users u
JOIN user_wallets w ON w.user_id = u.id
WHERE u.id = $1 AND u.deleted_at IS NULL;
