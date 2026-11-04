-- name: InsertChatPollOptions :exec
INSERT INTO chat_poll_options (message_id, position, label)
SELECT sqlc.arg(message_id)::uuid, o.position::smallint, o.label
FROM (
  SELECT unnest(sqlc.arg(positions)::int[]) AS position, unnest(sqlc.arg(labels)::text[]) AS label
) AS o;

-- name: ListChatPolls :many
SELECT o.message_id, o.position, o.label,
  (SELECT count(*) FROM chat_poll_votes v WHERE v.message_id = o.message_id AND v.position = o.position)::int AS votes,
  EXISTS (
    SELECT 1 FROM chat_poll_votes m
    WHERE m.message_id = o.message_id AND m.position = o.position AND m.user_id = sqlc.arg(viewer)::uuid
  )::bool AS mine
FROM chat_poll_options o
WHERE o.message_id = ANY(sqlc.arg(message_ids)::uuid[])
ORDER BY o.message_id, o.position;

-- name: GetChatPoll :one
SELECT a.payload
FROM chat_message_attachments a
JOIN cabal_messages m ON m.id = a.message_id AND m.deleted_at IS NULL
WHERE a.message_id = sqlc.arg(message_id) AND a.cabal_id = sqlc.arg(cabal_id) AND a.kind = 'poll';

-- name: UpsertChatPollVote :execrows
INSERT INTO chat_poll_votes (message_id, user_id, position, created_at, updated_at)
SELECT o.message_id, sqlc.arg(user_id)::uuid, o.position, sqlc.arg(at)::timestamptz, sqlc.arg(at)::timestamptz
FROM chat_poll_options o
WHERE o.message_id = sqlc.arg(message_id) AND o.position = sqlc.arg(position)::smallint
ON CONFLICT (message_id, user_id) DO UPDATE SET position = EXCLUDED.position, updated_at = EXCLUDED.updated_at;

-- name: DeleteChatPollVote :execrows
DELETE FROM chat_poll_votes WHERE message_id = sqlc.arg(message_id) AND user_id = sqlc.arg(user_id);

-- name: DeleteUserChatPollVotes :exec
DELETE FROM chat_poll_votes v
USING chat_message_attachments a
WHERE v.message_id = a.message_id AND a.cabal_id = sqlc.arg(cabal_id) AND v.user_id = sqlc.arg(user_id);
