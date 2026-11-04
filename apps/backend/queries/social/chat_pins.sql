-- name: LockChatPins :exec
SELECT lock_chat_pins(sqlc.arg(cabal_id)::uuid);

-- name: CountChatPins :one
SELECT count(*)::int FROM cabal_messages WHERE cabal_id = sqlc.arg(cabal_id) AND pinned_at IS NOT NULL;

-- name: PinChatMessage :execrows
UPDATE cabal_messages
SET pinned_at = sqlc.arg(at)::timestamptz, pinned_by = sqlc.arg(actor)::uuid
WHERE id = sqlc.arg(id) AND pinned_at IS NULL;

-- name: UnpinChatMessage :execrows
UPDATE cabal_messages
SET pinned_at = NULL, pinned_by = NULL
WHERE id = sqlc.arg(id) AND pinned_at IS NOT NULL;

-- name: ListChatPins :many
SELECT id, cabal_id, author_id, body, created_at, parent_id, also_in_channel, reply_count, last_reply_at, proposal_id, deleted_at, kind, moment, dedupe_key, pinned_at, pinned_by
FROM cabal_messages
WHERE cabal_id = sqlc.arg(cabal_id) AND pinned_at IS NOT NULL AND deleted_at IS NULL
ORDER BY pinned_at DESC, id DESC
LIMIT 3;
