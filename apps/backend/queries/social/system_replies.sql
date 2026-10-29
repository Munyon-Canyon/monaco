-- name: InsertSystemReply :execrows
INSERT INTO feed_comments (id, feed_object_id, author_kind, system_outcome, body, created_at)
VALUES (
  sqlc.arg(id), sqlc.arg(feed_object_id), 'system', sqlc.arg(outcome)::text, sqlc.arg(body)::text,
  sqlc.arg(created_at)::timestamptz
)
ON CONFLICT DO NOTHING;

-- name: ListSystemReplies :many
SELECT id, feed_object_id, author_id, parent_comment_id, reply_to_user_id, body, created_at, deleted_at, deleted_by,
  deleted_by_kind, reply_to_comment_id, author_kind, system_outcome
FROM feed_comments
WHERE feed_object_id = sqlc.arg(feed_object_id) AND author_kind = 'system' AND deleted_at IS NULL
ORDER BY created_at, id;
