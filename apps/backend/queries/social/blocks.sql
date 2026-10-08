-- name: InsertUserBlock :one
INSERT INTO user_blocks (id, blocker_id, blocked_id, created_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (blocker_id, blocked_id) DO NOTHING
RETURNING id;

-- name: DeleteUserBlock :one
DELETE FROM user_blocks
WHERE blocker_id = $1 AND blocked_id = $2
RETURNING id;
