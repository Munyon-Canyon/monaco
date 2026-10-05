-- name: InsertChatMessage :one
INSERT INTO cabal_messages (
  id, cabal_id, author_id, body, created_at, parent_id, also_in_channel, proposal_id
)
VALUES (
  sqlc.arg(id), sqlc.arg(cabal_id), sqlc.arg(author_id), sqlc.arg(body), sqlc.arg(created_at),
  sqlc.narg(parent_id), sqlc.arg(also_in_channel), sqlc.narg(proposal_id)
)
RETURNING id, cabal_id, author_id, body, created_at, parent_id, also_in_channel, reply_count, last_reply_at,
  proposal_id, deleted_at;

-- name: LockChatParent :one
SELECT id, cabal_id, parent_id, deleted_at
FROM cabal_messages
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: BumpChatReplyCount :execrows
UPDATE cabal_messages
SET reply_count = reply_count + 1, last_reply_at = sqlc.arg(at)
WHERE id = sqlc.arg(id);

-- name: SoftDeleteChatMessage :execrows
UPDATE cabal_messages
SET deleted_at = sqlc.arg(at)
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: FindChatMessage :one
SELECT id, cabal_id, author_id, body, created_at, parent_id, also_in_channel, reply_count, last_reply_at,
  proposal_id, deleted_at
FROM cabal_messages
WHERE id = sqlc.arg(id);
