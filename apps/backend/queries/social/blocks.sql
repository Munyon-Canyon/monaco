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
SELECT id, blocked_id, created_at FROM user_blocks
WHERE blocker_id = sqlc.arg(blocker_id)::uuid
  AND (
    NOT sqlc.arg(has_cursor)::boolean
    OR (created_at, id) < (sqlc.arg(after_at)::timestamptz, sqlc.arg(after_id)::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::int;

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

-- name: LockUserPair :exec
SELECT pg_advisory_xact_lock(hashtextextended('user-pair:' || least(sqlc.arg(user_a)::uuid, sqlc.arg(user_b)::uuid)::text || greatest(sqlc.arg(user_a)::uuid, sqlc.arg(user_b)::uuid)::text, 0));
