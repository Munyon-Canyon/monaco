-- name: InsertCommentAttachment :exec
INSERT INTO comment_attachments (comment_id, feed_object_id, kind, payload, created_at)
VALUES (sqlc.arg(comment_id), sqlc.arg(feed_object_id), sqlc.arg(kind), sqlc.arg(payload), sqlc.arg(created_at));
