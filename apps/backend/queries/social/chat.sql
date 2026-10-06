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

-- name: BumpChatReplies :one
UPDATE cabal_messages
SET reply_count = reply_count + 1, last_reply_at = sqlc.arg(at)::timestamptz
WHERE id = sqlc.arg(id) AND parent_id IS NULL
RETURNING id, reply_count, last_reply_at;

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

-- name: MarkChatSeen :one
INSERT INTO chat_seen (cabal_id, user_id, last_seen_at)
VALUES (sqlc.arg(cabal_id), sqlc.arg(user_id), sqlc.arg(now)::timestamptz)
ON CONFLICT (cabal_id, user_id) DO UPDATE
SET last_seen_at = GREATEST(chat_seen.last_seen_at, EXCLUDED.last_seen_at)
RETURNING last_seen_at;

-- name: ClaimSeenPublish :one
WITH newest AS (
  SELECT id, author_id, created_at
  FROM cabal_messages
  WHERE cabal_id = sqlc.arg(cabal_id)
    AND (parent_id IS NULL OR also_in_channel)
    AND deleted_at IS NULL
  ORDER BY created_at DESC, id DESC
  LIMIT 1
), claim AS (
  UPDATE chat_seen
  SET seen_published_at = sqlc.arg(now)::timestamptz
  WHERE chat_seen.cabal_id = sqlc.arg(cabal_id) AND chat_seen.user_id = sqlc.arg(user_id)
    AND (seen_published_at IS NULL OR seen_published_at <= sqlc.arg(now)::timestamptz - interval '5 seconds')
    AND EXISTS (SELECT 1 FROM newest)
  RETURNING 1
)
SELECT newest.id AS message_id, (
  SELECT count(*)::int
  FROM chat_seen
  WHERE chat_seen.cabal_id = sqlc.arg(cabal_id)
    AND chat_seen.user_id <> newest.author_id
    AND chat_seen.last_seen_at >= newest.created_at
) AS seen_count
FROM newest
WHERE EXISTS (SELECT 1 FROM claim);

-- name: NewestSeenCount :one
WITH newest AS (
  SELECT id, author_id, created_at
  FROM cabal_messages
  WHERE cabal_id = sqlc.arg(cabal_id)
    AND (parent_id IS NULL OR also_in_channel)
    AND deleted_at IS NULL
  ORDER BY created_at DESC, id DESC
  LIMIT 1
)
SELECT newest.id AS message_id, (
  SELECT count(*)::int
  FROM chat_seen
  WHERE chat_seen.cabal_id = sqlc.arg(cabal_id)
    AND chat_seen.user_id <> newest.author_id
    AND chat_seen.last_seen_at >= newest.created_at
) AS seen_count
FROM newest;

-- name: ListSeenBy :many
SELECT user_id
FROM chat_seen
WHERE cabal_id = sqlc.arg(cabal_id)
  AND user_id <> sqlc.arg(author_id)
  AND last_seen_at >= sqlc.arg(message_at)::timestamptz
ORDER BY last_seen_at DESC, user_id;

-- name: UnreadCounts :many
SELECT c.id::uuid AS cabal_id, (
  SELECT count(*)::int
  FROM (
    SELECT 1
    FROM cabal_messages m
    WHERE m.cabal_id = c.id
      AND (m.parent_id IS NULL OR m.also_in_channel)
      AND m.deleted_at IS NULL
      AND m.author_id <> sqlc.arg(user_id)
      AND m.created_at > coalesce(s.last_seen_at, '-infinity'::timestamptz)
    LIMIT 100
  ) AS capped
) AS unread
FROM unnest(sqlc.arg(cabal_ids)::uuid[]) AS c(id)
LEFT JOIN chat_seen s ON s.cabal_id = c.id AND s.user_id = sqlc.arg(user_id);

-- name: DeleteChatSeen :exec
DELETE FROM chat_seen
WHERE cabal_id = sqlc.arg(cabal_id) AND user_id = sqlc.arg(user_id);
