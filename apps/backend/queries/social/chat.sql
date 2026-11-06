-- name: LockChatMessage :one
SELECT id, cabal_id, author_id, parent_id, kind, (deleted_at IS NOT NULL)::bool AS deleted, pinned_at, pinned_by
FROM cabal_messages
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: InsertChatMessage :one
INSERT INTO cabal_messages (id, cabal_id, author_id, body, created_at, parent_id, also_in_channel)
VALUES (
  sqlc.arg(id), sqlc.arg(cabal_id), sqlc.arg(author_id)::uuid, NULLIF(sqlc.arg(body)::text, ''), sqlc.arg(created_at), NULLIF(sqlc.arg(parent_id)::uuid, '00000000-0000-0000-0000-000000000000'),
  sqlc.arg(also_in_channel)
)
RETURNING id, cabal_id, author_id, body, created_at, parent_id, also_in_channel;

-- name: InsertChatCommentCopy :one
INSERT INTO cabal_messages (id, cabal_id, author_id, body, created_at, parent_id, also_in_channel, comment_id)
VALUES (
  sqlc.arg(id), sqlc.arg(cabal_id), sqlc.arg(author_id)::uuid, NULLIF(sqlc.arg(body)::text, ''), sqlc.arg(created_at),
  sqlc.arg(parent_id)::uuid, true, sqlc.arg(comment_id)::uuid
)
RETURNING id, cabal_id, author_id, body, created_at, parent_id, also_in_channel, comment_id;

-- name: SoftDeleteChatCopyOfComment :many
UPDATE cabal_messages
SET deleted_at = sqlc.arg(at)::timestamptz, pinned_at = NULL, pinned_by = NULL
WHERE comment_id = sqlc.arg(comment_id)::uuid AND deleted_at IS NULL
RETURNING id, cabal_id;

-- name: ProposalThreadParticipants :many
SELECT DISTINCT p.author_id
FROM (
  SELECT m.author_id FROM cabal_messages m WHERE m.id = sqlc.arg(card_id) AND m.deleted_at IS NULL
  UNION ALL
  SELECT c.author_id FROM feed_comments c
  WHERE c.feed_object_id = sqlc.arg(feed_object_id) AND c.author_kind = 'user' AND c.deleted_at IS NULL
) AS p
WHERE p.author_id IS NOT NULL
ORDER BY p.author_id;

-- name: InsertChatMoment :one
INSERT INTO cabal_messages (id, cabal_id, kind, moment, dedupe_key, created_at)
VALUES (sqlc.arg(id), sqlc.arg(cabal_id), 'system', sqlc.arg(moment)::jsonb, sqlc.arg(dedupe_key)::text, sqlc.arg(created_at))
ON CONFLICT (cabal_id, dedupe_key) WHERE dedupe_key IS NOT NULL DO UPDATE SET dedupe_key = cabal_messages.dedupe_key
RETURNING id, cabal_id, created_at;

-- name: InsertChatProposal :one
INSERT INTO cabal_messages (id, cabal_id, author_id, body, kind, proposal_id, dedupe_key, created_at)
VALUES (
  sqlc.arg(id), sqlc.arg(cabal_id), sqlc.arg(author_id)::uuid, NULLIF(sqlc.arg(body)::text, ''), 'proposal',
  sqlc.arg(proposal_id)::uuid, sqlc.arg(dedupe_key)::text, sqlc.arg(created_at)
)
ON CONFLICT (cabal_id, dedupe_key) WHERE dedupe_key IS NOT NULL DO UPDATE SET dedupe_key = cabal_messages.dedupe_key
RETURNING id, cabal_id, created_at;

-- name: BumpChatReplies :one
UPDATE cabal_messages
SET reply_count = reply_count + 1, last_reply_at = sqlc.arg(at)::timestamptz
WHERE id = sqlc.arg(id) AND parent_id IS NULL
RETURNING id, reply_count, last_reply_at;

-- name: DropChatReply :one
UPDATE cabal_messages
SET reply_count = GREATEST(reply_count - 1, 0)
WHERE id = sqlc.arg(id) AND parent_id IS NULL
RETURNING id, reply_count, last_reply_at;

-- name: SoftDeleteChatMessage :execrows
UPDATE cabal_messages
SET deleted_at = sqlc.arg(at)::timestamptz, pinned_at = NULL, pinned_by = NULL
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: GetChatCursor :one
SELECT created_at, id
FROM cabal_messages
WHERE id = sqlc.arg(id) AND cabal_id = sqlc.arg(cabal_id);

-- name: ListChatChannel :many
SELECT id, cabal_id, author_id, body, created_at, parent_id, also_in_channel, reply_count, last_reply_at, proposal_id, deleted_at, kind, moment, pinned_at, pinned_by, comment_id,
  blocked_author.hit AS blocked
FROM cabal_messages
CROSS JOIN LATERAL (
  SELECT EXISTS (
    SELECT 1 FROM user_blocks b
    WHERE b.blocker_id = sqlc.arg(viewer)::uuid AND b.blocked_id = cabal_messages.author_id
  ) AS hit
) AS blocked_author
WHERE cabal_id = sqlc.arg(cabal_id)
  AND (parent_id IS NULL OR also_in_channel)
  AND (kind <> 'system' OR sqlc.arg(moments)::bool)
  AND (deleted_at IS NULL OR (parent_id IS NULL AND reply_count > 0))
  AND (
    NOT blocked_author.hit
    OR (parent_id IS NULL AND reply_count > 0)
  )
  AND (
    NOT sqlc.arg(has_before)::bool
    OR (created_at, id) < (sqlc.arg(before_at)::timestamptz, sqlc.arg(before_id)::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::int;

-- name: ListChatChannelAfter :many
SELECT id, cabal_id, author_id, body, created_at, parent_id, also_in_channel, reply_count, last_reply_at, proposal_id, deleted_at, kind, moment, pinned_at, pinned_by, comment_id,
  blocked_author.hit AS blocked
FROM cabal_messages
CROSS JOIN LATERAL (
  SELECT EXISTS (
    SELECT 1 FROM user_blocks b
    WHERE b.blocker_id = sqlc.arg(viewer)::uuid AND b.blocked_id = cabal_messages.author_id
  ) AS hit
) AS blocked_author
WHERE cabal_id = sqlc.arg(cabal_id)
  AND (parent_id IS NULL OR also_in_channel)
  AND (kind <> 'system' OR sqlc.arg(moments)::bool)
  AND (deleted_at IS NULL OR (parent_id IS NULL AND reply_count > 0))
  AND (
    NOT blocked_author.hit
    OR (parent_id IS NULL AND reply_count > 0)
  )
  AND (created_at, id) > (sqlc.arg(after_at)::timestamptz, sqlc.arg(after_id)::uuid)
ORDER BY created_at, id
LIMIT sqlc.arg(row_limit)::int;

-- name: GetChatMessage :one
SELECT id, cabal_id, author_id, body, created_at, parent_id, also_in_channel, reply_count, last_reply_at, proposal_id, deleted_at, kind, moment, dedupe_key, pinned_at, pinned_by, comment_id
FROM cabal_messages
WHERE id = sqlc.arg(id) AND cabal_id = sqlc.arg(cabal_id);

-- name: ListChatReplies :many
SELECT id, cabal_id, author_id, body, created_at, parent_id, also_in_channel, reply_count, last_reply_at, proposal_id, deleted_at, kind, moment, dedupe_key, pinned_at, pinned_by, comment_id
FROM cabal_messages
WHERE parent_id = sqlc.arg(parent_id)::uuid
  AND deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM user_blocks b
    WHERE b.blocker_id = sqlc.arg(viewer)::uuid AND b.blocked_id = cabal_messages.author_id
  )
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
    AND kind <> 'system'
    AND deleted_at IS NULL
  ORDER BY created_at DESC, id DESC
  LIMIT 1
), claim AS (
  UPDATE chat_seen
  SET seen_published_at = sqlc.arg(now)::timestamptz
  WHERE chat_seen.cabal_id = sqlc.arg(cabal_id) AND chat_seen.user_id = sqlc.arg(user_id)
    AND (
      seen_published_at IS NULL
      OR seen_published_at <= sqlc.arg(now)::timestamptz - interval '5 seconds'
      OR seen_published_at < (SELECT created_at FROM newest)
    )
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
    AND kind <> 'system'
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
      AND m.kind <> 'system'
      AND m.author_id <> sqlc.arg(user_id)::uuid
      AND NOT EXISTS (
        SELECT 1 FROM user_blocks b
        WHERE b.blocker_id = sqlc.arg(user_id)::uuid AND b.blocked_id = m.author_id
      )
      AND m.created_at > coalesce(s.last_seen_at, '-infinity'::timestamptz)
    LIMIT 100
  ) AS capped
) AS unread
FROM unnest(sqlc.arg(cabal_ids)::uuid[]) AS c(id)
LEFT JOIN chat_seen s ON s.cabal_id = c.id AND s.user_id = sqlc.arg(user_id);

-- name: DeleteChatSeen :exec
DELETE FROM chat_seen
WHERE cabal_id = sqlc.arg(cabal_id) AND user_id = sqlc.arg(user_id);

-- name: ThreadParticipants :many
SELECT DISTINCT author_id
FROM cabal_messages
WHERE (id = sqlc.arg(parent_id) OR parent_id = sqlc.arg(parent_id))
  AND deleted_at IS NULL
ORDER BY author_id;

-- name: GetChatMessageByID :one
SELECT id, cabal_id, kind FROM cabal_messages WHERE id = sqlc.arg(id);

-- name: SeedChatSeen :exec
INSERT INTO chat_seen (cabal_id, user_id, last_seen_at)
VALUES (
  sqlc.arg(cabal_id), sqlc.arg(user_id),
  coalesce((
    SELECT max(m.created_at)
    FROM cabal_messages m
    WHERE m.cabal_id = sqlc.arg(cabal_id) AND m.created_at <= sqlc.arg(joined_at)::timestamptz
  ), sqlc.arg(joined_at)::timestamptz)
)
ON CONFLICT (cabal_id, user_id) DO UPDATE
SET last_seen_at = GREATEST(chat_seen.last_seen_at, EXCLUDED.last_seen_at);

-- name: GetProposalCommentCursor :one
SELECT created_at, id
FROM feed_comments
WHERE id = sqlc.arg(id) AND feed_object_id = sqlc.arg(feed_object_id);

-- name: ListProposalChatReplies :many
SELECT c.id, c.feed_object_id, c.author_id, c.parent_comment_id, c.reply_to_user_id, c.reply_to_comment_id, c.body,
  c.created_at,
  r.id AS quote_id, r.author_id AS quote_author_id, r.parent_comment_id AS quote_parent_id, r.body AS quote_body,
  (r.deleted_at IS NOT NULL)::bool AS quote_deleted,
  EXISTS (SELECT 1 FROM comment_attachments qa WHERE qa.comment_id = r.id)::bool AS quote_has_attachment,
  a.kind AS attachment_kind, a.payload AS attachment_payload,
  EXISTS (
    SELECT 1 FROM user_blocks b WHERE b.blocker_id = sqlc.arg(viewer)::uuid AND b.blocked_id = r.author_id
  )::bool AS quote_blocked
FROM feed_comments c
LEFT JOIN feed_comments r ON r.feed_object_id = c.feed_object_id AND r.id = c.reply_to_comment_id
LEFT JOIN comment_attachments a ON a.comment_id = c.id
WHERE c.feed_object_id = sqlc.arg(feed_object_id)
  AND c.author_kind = 'user'
  AND c.deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM user_blocks b WHERE b.blocker_id = sqlc.arg(viewer)::uuid AND b.blocked_id = c.author_id
  )
  AND (
    NOT sqlc.arg(has_before)::bool
    OR (c.created_at, c.id) < (sqlc.arg(before_at)::timestamptz, sqlc.arg(before_id)::uuid)
  )
ORDER BY c.created_at DESC, c.id DESC
LIMIT sqlc.arg(row_limit)::int;

-- name: ProposalChatReplyStats :many
SELECT o.ref_id AS proposal_id, count(c.id)::int AS reply_count, max(c.created_at)::timestamptz AS last_reply_at
FROM feed_objects AS o
JOIN feed_comments AS c ON c.feed_object_id = o.id AND c.deleted_at IS NULL AND c.author_kind = 'user'
WHERE o.ref_type = 'proposals' AND o.kind = 'proposal' AND o.ref_id = ANY(sqlc.arg(proposal_ids)::uuid[])
GROUP BY o.ref_id;

-- name: GetProposalCardOfComment :one
SELECT m.id AS card_id, m.cabal_id, o.ref_id AS proposal_id, c.author_kind, live.reply_count, live.last_reply_at
FROM feed_comments c
JOIN feed_objects o ON o.id = c.feed_object_id AND o.ref_type = 'proposals' AND o.kind = 'proposal'
JOIN cabal_messages m ON m.proposal_id = o.ref_id AND m.kind = 'proposal'
CROSS JOIN LATERAL (
  SELECT count(*)::int AS reply_count, coalesce(max(l.created_at), 'epoch'::timestamptz)::timestamptz AS last_reply_at
  FROM feed_comments l
  WHERE l.feed_object_id = o.id AND l.deleted_at IS NULL AND l.author_kind = 'user'
) AS live
WHERE c.id = sqlc.arg(comment_id);
