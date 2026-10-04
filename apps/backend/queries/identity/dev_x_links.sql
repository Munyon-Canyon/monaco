-- name: UpsertDevXLink :exec
INSERT INTO dev_x_links (user_id, x_user_id, x_username, created_at)
VALUES (sqlc.arg(user_id), sqlc.arg(x_user_id), sqlc.arg(x_username), sqlc.arg(now))
ON CONFLICT (user_id) DO UPDATE SET x_user_id = EXCLUDED.x_user_id, x_username = EXCLUDED.x_username;

-- name: DeleteDevXLink :exec
DELETE FROM dev_x_links WHERE user_id = $1;

-- name: DevXLinkByPrivyUserID :one
SELECT d.x_user_id, d.x_username
FROM dev_x_links d
JOIN users u ON u.id = d.user_id
WHERE u.privy_user_id = $1;
