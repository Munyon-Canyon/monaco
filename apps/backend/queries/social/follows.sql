-- name: InsertFollow :one
INSERT INTO follows (id, follower_id, followee_id, source, created_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (follower_id, followee_id) WHERE deleted_at IS NULL DO NOTHING
RETURNING id;

-- name: RemoveFollow :one
UPDATE follows SET deleted_at = sqlc.arg(removed_at)::timestamptz
WHERE follower_id = $1 AND followee_id = $2 AND deleted_at IS NULL
RETURNING id;
