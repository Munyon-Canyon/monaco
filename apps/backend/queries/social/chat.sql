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

-- name: GetChatCursor :one
SELECT created_at, id
FROM cabal_messages
WHERE id = sqlc.arg(id) AND cabal_id = sqlc.arg(cabal_id);

-- name: ListChatChannel :many
SELECT id, cabal_id, author_id, body, created_at, parent_id, also_in_channel, reply_count, last_reply_at, proposal_id, deleted_at
FROM cabal_messages
WHERE cabal_id = sqlc.arg(cabal_id)
  AND (parent_id IS NULL OR also_in_channel)
  AND (deleted_at IS NULL OR (parent_id IS NULL AND reply_count > 0))
  AND (
    NOT sqlc.arg(has_before)::bool
    OR (created_at, id) < (sqlc.arg(before_at)::timestamptz, sqlc.arg(before_id)::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::int;

-- name: ListChatChannelAfter :many
SELECT id, cabal_id, author_id, body, created_at, parent_id, also_in_channel, reply_count, last_reply_at, proposal_id, deleted_at
FROM cabal_messages
WHERE cabal_id = sqlc.arg(cabal_id)
  AND (parent_id IS NULL OR also_in_channel)
  AND (deleted_at IS NULL OR (parent_id IS NULL AND reply_count > 0))
  AND (created_at, id) > (sqlc.arg(after_at)::timestamptz, sqlc.arg(after_id)::uuid)
ORDER BY created_at, id
LIMIT sqlc.arg(row_limit)::int;

-- name: GetChatMessage :one
SELECT id, cabal_id, author_id, body, created_at, parent_id, also_in_channel, reply_count, last_reply_at, proposal_id, deleted_at
FROM cabal_messages
WHERE id = sqlc.arg(id) AND cabal_id = sqlc.arg(cabal_id);

-- name: ListChatReplies :many
SELECT id, cabal_id, author_id, body, created_at, parent_id, also_in_channel, reply_count, last_reply_at, proposal_id, deleted_at
FROM cabal_messages
WHERE parent_id = sqlc.arg(parent_id)::uuid
  AND deleted_at IS NULL
  AND (
    NOT sqlc.arg(has_before)::bool
    OR (created_at, id) < (sqlc.arg(before_at)::timestamptz, sqlc.arg(before_id)::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::int;
