-- name: AddChatReaction :one
INSERT INTO chat_message_reactions (message_id, cabal_id, user_id, emoji, created_at)
VALUES (sqlc.arg(message_id), sqlc.arg(cabal_id), sqlc.arg(user_id), sqlc.arg(emoji), sqlc.arg(created_at))
ON CONFLICT (message_id, user_id, emoji) DO NOTHING
RETURNING created_at;

-- name: RemoveChatReaction :execrows
DELETE FROM chat_message_reactions
WHERE message_id = sqlc.arg(message_id) AND user_id = sqlc.arg(user_id) AND emoji = sqlc.arg(emoji);

-- name: CountUserChatReactions :one
SELECT count(*)::int FROM chat_message_reactions
WHERE message_id = sqlc.arg(message_id) AND user_id = sqlc.arg(user_id);

-- name: ChatReactionSummaries :many
WITH counted AS (
  SELECT r.message_id, r.emoji, count(*)::int AS count, bool_or(r.user_id = sqlc.arg(viewer)::uuid) AS mine,
    min(r.created_at) AS first_at
  FROM chat_message_reactions r
  JOIN cabal_messages m ON m.id = r.message_id AND m.deleted_at IS NULL
  WHERE r.message_id = ANY(sqlc.arg(message_ids)::uuid[])
  GROUP BY r.message_id, r.emoji
), ranked AS (
  SELECT c.message_id, c.emoji, c.count, c.mine,
    row_number() OVER (PARTITION BY c.message_id ORDER BY c.count DESC, c.first_at, c.emoji) AS rank,
    sum(c.count) OVER (PARTITION BY c.message_id)::int AS total
  FROM counted c
)
SELECT ranked.message_id, ranked.emoji, ranked.count, ranked.mine::bool AS mine, ranked.total
FROM ranked
WHERE ranked.rank <= 20
ORDER BY ranked.message_id, ranked.rank;

-- name: DeleteChatReactionsOf :exec
DELETE FROM chat_message_reactions WHERE cabal_id = sqlc.arg(cabal_id) AND user_id = sqlc.arg(user_id);

-- name: LiveChatMessageAuthor :one
SELECT author_id FROM cabal_messages
WHERE id = sqlc.arg(id) AND cabal_id = sqlc.arg(cabal_id) AND deleted_at IS NULL
FOR SHARE;
