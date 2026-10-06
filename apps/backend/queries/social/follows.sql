-- name: InsertFollow :one
INSERT INTO follows (id, follower_id, followee_id, source, created_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (follower_id, followee_id) WHERE deleted_at IS NULL DO NOTHING
RETURNING id;

-- name: RemoveFollow :one
UPDATE follows SET deleted_at = sqlc.arg(removed_at)::timestamptz
WHERE follower_id = $1 AND followee_id = $2 AND deleted_at IS NULL
RETURNING id;

-- name: ListFollowers :many
SELECT follower_id, created_at FROM follows
WHERE followee_id = sqlc.arg(user_id)::uuid AND deleted_at IS NULL
  AND (
    NOT sqlc.arg(has_cursor)::boolean
    OR (created_at, follower_id) < (sqlc.arg(after_at)::timestamptz, sqlc.arg(after_id)::uuid)
  )
ORDER BY created_at DESC, follower_id DESC
LIMIT sqlc.arg(row_limit)::int;

-- name: ListFollowing :many
SELECT followee_id, created_at FROM follows
WHERE follower_id = sqlc.arg(user_id)::uuid AND deleted_at IS NULL
  AND (
    NOT sqlc.arg(has_cursor)::boolean
    OR (created_at, followee_id) < (sqlc.arg(after_at)::timestamptz, sqlc.arg(after_id)::uuid)
  )
ORDER BY created_at DESC, followee_id DESC
LIMIT sqlc.arg(row_limit)::int;

-- name: FollowedAmong :many
SELECT followee_id FROM follows
WHERE follower_id = sqlc.arg(follower_id)::uuid AND followee_id = ANY(sqlc.arg(followee_ids)::uuid[])
  AND deleted_at IS NULL;
