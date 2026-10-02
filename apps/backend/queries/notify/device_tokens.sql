-- name: RegisterToken :exec
INSERT INTO device_tokens (id, user_id, token, environment, created_at, last_seen_at)
VALUES ($1, $2, $3, $4, $5, $5)
ON CONFLICT (token) DO UPDATE
SET user_id = excluded.user_id,
    environment = excluded.environment,
    last_seen_at = excluded.last_seen_at,
    disabled_at = NULL;

-- name: DeleteOwnToken :one
DELETE FROM device_tokens
WHERE token = $1 AND user_id = $2
RETURNING environment;

-- name: ActiveTokensForUser :many
SELECT token, environment FROM device_tokens
WHERE user_id = $1 AND disabled_at IS NULL
ORDER BY last_seen_at DESC, token;

-- name: DisableToken :execrows
UPDATE device_tokens SET disabled_at = sqlc.arg(disabled_at)::timestamptz
WHERE token = sqlc.arg(token) AND disabled_at IS NULL;
