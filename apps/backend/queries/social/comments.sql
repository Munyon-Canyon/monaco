-- name: GetCommentItem :one
SELECT id, kind, ref_type, ref_id, cabal_id, actor_id, payload
FROM feed_objects
WHERE id = sqlc.arg(id);

-- name: GetProposalFeedItem :one
SELECT id
FROM feed_objects
WHERE ref_type = 'proposals' AND ref_id = sqlc.arg(proposal_id)::uuid AND kind = 'proposal';

-- name: GetProposalFeedSource :one
SELECT cabal_id, cabal_name, actor_id, asset_id, symbol, body, payload
FROM feed_objects
WHERE ref_type = 'proposals' AND ref_id = sqlc.arg(proposal_id)::uuid AND kind = 'proposal';

-- name: InsertFeedReply :exec
INSERT INTO feed_objects (
  id, kind, ref_type, ref_id, cabal_id, cabal_name, actor_id, asset_id, symbol, title, body, payload,
  created_at, updated_at
)
SELECT
  sqlc.arg(id)::uuid, 'reply', 'feed_comments', sqlc.arg(comment_id)::uuid, o.cabal_id, o.cabal_name,
  sqlc.arg(actor_id)::uuid, o.asset_id, o.symbol, sqlc.arg(title)::text, sqlc.arg(body)::text,
  sqlc.arg(payload)::jsonb, sqlc.arg(at)::timestamptz, sqlc.arg(at)::timestamptz
FROM feed_objects o
WHERE o.id = sqlc.arg(feed_object_id)::uuid
ON CONFLICT (ref_type, ref_id, kind) DO NOTHING;

-- name: DeleteFeedCommentItem :execrows
WITH item AS (
  SELECT id FROM feed_objects WHERE ref_type = 'feed_comments' AND ref_id = sqlc.arg(comment_id)::uuid
), unliked AS (
  DELETE FROM feed_likes WHERE feed_object_id IN (SELECT id FROM item)
)
DELETE FROM feed_objects WHERE id IN (SELECT id FROM item);

-- name: GetCommentTarget :one
SELECT c.id, c.author_id, c.parent_comment_id, c.body, (c.deleted_at IS NOT NULL)::bool AS deleted,
  EXISTS (
    SELECT 1 FROM user_blocks b WHERE b.blocker_id = sqlc.arg(viewer)::uuid AND b.blocked_id = c.author_id
  )::bool AS blocked
FROM feed_comments c
WHERE c.id = sqlc.arg(id) AND c.feed_object_id = sqlc.arg(feed_object_id);

-- name: InsertComment :one
INSERT INTO feed_comments (
  id, feed_object_id, author_id, parent_comment_id, reply_to_user_id, reply_to_comment_id, body, created_at
)
VALUES (
  sqlc.arg(id), sqlc.arg(feed_object_id), sqlc.arg(author_id),
  NULLIF(sqlc.arg(parent_comment_id)::uuid, '00000000-0000-0000-0000-000000000000'),
  NULLIF(sqlc.arg(reply_to_user_id)::uuid, '00000000-0000-0000-0000-000000000000'),
  NULLIF(sqlc.arg(reply_to_comment_id)::uuid, '00000000-0000-0000-0000-000000000000'),
  sqlc.arg(body), sqlc.arg(created_at)
)
RETURNING id, feed_object_id, author_id, parent_comment_id, reply_to_user_id, body, created_at, deleted_at, deleted_by,
  deleted_by_kind, reply_to_comment_id;

-- name: BumpCommentCount :execrows
UPDATE feed_objects
SET comment_count = comment_count + sqlc.arg(delta)::int
WHERE id = sqlc.arg(id);

-- name: LockComment :one
SELECT id, feed_object_id, author_id, (deleted_at IS NOT NULL)::bool AS deleted
FROM feed_comments
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: SoftDeleteComment :execrows
UPDATE feed_comments
SET deleted_at = sqlc.arg(at)::timestamptz, deleted_by = sqlc.arg(by)::uuid,
  deleted_by_kind = sqlc.arg(kind)::text
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: ListCommentThreads :many
WITH tops AS (
  SELECT t.id
  FROM feed_comments t
  WHERE t.feed_object_id = sqlc.arg(feed_object_id)
    AND t.parent_comment_id IS NULL
    AND NOT EXISTS (
      SELECT 1 FROM user_blocks b WHERE b.blocker_id = sqlc.arg(viewer)::uuid AND b.blocked_id = t.author_id
    )
    AND (
      NOT sqlc.arg(has_cursor)::bool
      OR (t.created_at, t.id) > (sqlc.arg(after_at)::timestamptz, sqlc.arg(after_id)::uuid)
    )
  ORDER BY t.created_at, t.id
  LIMIT sqlc.arg(row_limit)::int
)
SELECT c.id, c.feed_object_id, c.author_id, c.parent_comment_id, c.reply_to_user_id, c.reply_to_comment_id, c.body,
  c.created_at, c.deleted_at, c.deleted_by, c.deleted_by_kind,
  r.id AS quote_id, r.author_id AS quote_author_id, r.parent_comment_id AS quote_parent_id, r.body AS quote_body,
  (r.deleted_at IS NOT NULL)::bool AS quote_deleted,
  EXISTS (
    SELECT 1 FROM user_blocks b WHERE b.blocker_id = sqlc.arg(viewer)::uuid AND b.blocked_id = r.author_id
  )::bool AS quote_blocked
FROM feed_comments c
LEFT JOIN feed_comments r ON r.feed_object_id = c.feed_object_id AND r.id = c.reply_to_comment_id
WHERE c.feed_object_id = sqlc.arg(feed_object_id)
  AND (c.id IN (SELECT id FROM tops) OR c.parent_comment_id IN (SELECT id FROM tops))
  AND NOT EXISTS (
    SELECT 1 FROM user_blocks b WHERE b.blocker_id = sqlc.arg(viewer)::uuid AND b.blocked_id = c.author_id
  )
ORDER BY c.created_at, c.id;

-- name: GetComment :one
SELECT c.id, o.kind, o.cabal_id
FROM feed_comments c
JOIN feed_objects o ON o.id = c.feed_object_id
WHERE c.id = sqlc.arg(id);
