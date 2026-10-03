-- name: LockChatMessage :one
SELECT id, cabal_id, author_id, parent_id, (deleted_at IS NOT NULL)::bool AS deleted
FROM cabal_messages
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: InsertChatMessage :one
INSERT INTO cabal_messages (id, cabal_id, author_id, body, created_at, parent_id, also_in_channel)
VALUES (
  sqlc.arg(id), sqlc.arg(cabal_id), sqlc.arg(author_id), sqlc.arg(body), sqlc.arg(created_at), NULLIF(sqlc.arg(parent_id)::uuid, '00000000-0000-0000-0000-000000000000'),
  sqlc.arg(also_in_channel)
)
RETURNING id, cabal_id, author_id, body, created_at, parent_id, also_in_channel;

-- name: BumpChatReplies :execrows
UPDATE cabal_messages
SET reply_count = reply_count + 1, last_reply_at = sqlc.arg(at)::timestamptz
WHERE id = sqlc.arg(id) AND parent_id IS NULL;

-- name: SoftDeleteChatMessage :execrows
UPDATE cabal_messages
SET deleted_at = sqlc.arg(at)::timestamptz
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;
