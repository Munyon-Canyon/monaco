-- name: GetCommentItem :one
SELECT id, kind, ref_type, ref_id, cabal_id, actor_id
FROM feed_objects
WHERE id = sqlc.arg(id);

-- name: GetCommentTarget :one
SELECT id, author_id, parent_comment_id, (deleted_at IS NOT NULL)::bool AS deleted
FROM feed_comments
WHERE id = sqlc.arg(id) AND feed_object_id = sqlc.arg(feed_object_id);

-- name: InsertComment :one
INSERT INTO feed_comments (id, feed_object_id, author_id, parent_comment_id, reply_to_user_id, body, created_at)
VALUES (
  sqlc.arg(id), sqlc.arg(feed_object_id), sqlc.arg(author_id),
  NULLIF(sqlc.arg(parent_comment_id)::uuid, '00000000-0000-0000-0000-000000000000'),
  NULLIF(sqlc.arg(reply_to_user_id)::uuid, '00000000-0000-0000-0000-000000000000'),
  sqlc.arg(body), sqlc.arg(created_at)
)
RETURNING id, feed_object_id, author_id, parent_comment_id, reply_to_user_id, body, created_at;

-- name: BumpCommentCount :execrows
UPDATE feed_objects
SET comment_count = comment_count + sqlc.arg(delta)::int
WHERE id = sqlc.arg(id);
