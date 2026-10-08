-- name: InsertServiceToken :exec
INSERT INTO admin_service_tokens (id, name, token_hash, created_at, created_by)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(token_hash), sqlc.arg(created_at), sqlc.arg(created_by));

-- name: LiveServiceTokenName :one
SELECT name FROM admin_service_tokens WHERE token_hash = sqlc.arg(token_hash) AND revoked_at IS NULL;

-- name: RevokeServiceToken :execrows
UPDATE admin_service_tokens SET revoked_at = sqlc.arg(revoked_at)::timestamptz
WHERE name = sqlc.arg(name) AND revoked_at IS NULL;
