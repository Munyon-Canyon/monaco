-- name: InsertUserBlock :one
INSERT INTO user_blocks (id, blocker_id, blocked_id, created_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (blocker_id, blocked_id) DO NOTHING
RETURNING id;

-- name: DeleteUserBlock :one
DELETE FROM user_blocks
WHERE blocker_id = $1 AND blocked_id = $2
RETURNING id;

-- name: ListUserBlocks :many
SELECT blocked_id FROM user_blocks
WHERE blocker_id = $1
ORDER BY created_at DESC, id DESC
LIMIT 500;

-- name: IsBlockedEitherWay :one
SELECT EXISTS (
  SELECT 1 FROM user_blocks
  WHERE (blocker_id = sqlc.arg(user_a)::uuid AND blocked_id = sqlc.arg(user_b)::uuid)
     OR (blocker_id = sqlc.arg(user_b)::uuid AND blocked_id = sqlc.arg(user_a)::uuid)
);

-- name: IsBlocked :one
SELECT EXISTS (
  SELECT 1 FROM user_blocks
  WHERE blocker_id = sqlc.arg(blocker)::uuid AND blocked_id = sqlc.arg(blocked)::uuid
);
