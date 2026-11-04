-- name: InsertCommentAttachment :exec
INSERT INTO comment_attachments (comment_id, feed_object_id, kind, payload, created_at)
VALUES (sqlc.arg(comment_id), sqlc.arg(feed_object_id), sqlc.arg(kind), sqlc.arg(payload), sqlc.arg(created_at));

-- name: InsertChatAttachment :exec
INSERT INTO chat_message_attachments (message_id, cabal_id, kind, payload, created_at)
VALUES (sqlc.arg(message_id), sqlc.arg(cabal_id), sqlc.arg(kind), sqlc.arg(payload), sqlc.arg(created_at));

-- name: ListChatAttachments :many
SELECT a.message_id, a.kind, a.payload
FROM chat_message_attachments a
JOIN cabal_messages m ON m.id = a.message_id AND m.deleted_at IS NULL
WHERE a.message_id = ANY(sqlc.arg(message_ids)::uuid[]);
