-- name: GetCommentItem :one
SELECT id, kind, ref_type, ref_id, cabal_id, actor_id
FROM feed_objects
WHERE id = sqlc.arg(id);

-- name: GetProposalFeedItem :one
SELECT id
FROM feed_objects
WHERE ref_type = 'proposals' AND ref_id = sqlc.arg(proposal_id)::uuid AND kind = 'proposal';

-- name: GetCommentTarget :one
SELECT id, author_id, parent_comment_id, (deleted_at IS NOT NULL)::bool AS deleted
FROM feed_comments
WHERE id = sqlc.arg(id) AND feed_object_id = sqlc.arg(feed_object_id);

-- name: InsertComment :one
INSERT INTO feed_comments (id, feed_object_id, author_id, parent_comment_id, reply_to_user_id, body, created_at)
VALUES (
  sqlc.arg(id), sqlc.arg(feed_object_id), sqlc.arg(author_id),
  NULLIF(sqlc.arg(parent_comment_id)::uuid, '00000000-0000-0000-0000-000000000000'),
  NULLIF(sqlc.arg(reply_to_user_id)::uuid, '00000000-0000-0000-0000-000000000000'),
  sqlc.arg(body), sqlc.arg(created_at)
)
RETURNING id, feed_object_id, author_id, parent_comment_id, reply_to_user_id, body, created_at, deleted_at, deleted_by;

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
SET deleted_at = sqlc.arg(at)::timestamptz, deleted_by = sqlc.arg(by)::uuid
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: ListCommentThreads :many
WITH tops AS (
  SELECT t.id
  FROM feed_comments t
  WHERE t.feed_object_id = sqlc.arg(feed_object_id)
    AND t.parent_comment_id IS NULL
    AND (
      NOT sqlc.arg(has_cursor)::bool
      OR (t.created_at, t.id) > (sqlc.arg(after_at)::timestamptz, sqlc.arg(after_id)::uuid)
    )
  ORDER BY t.created_at, t.id
  LIMIT sqlc.arg(row_limit)::int
)
SELECT c.id, c.feed_object_id, c.author_id, c.parent_comment_id, c.reply_to_user_id, c.body, c.created_at,
  c.deleted_at, c.deleted_by
FROM feed_comments c
WHERE c.id IN (SELECT id FROM tops) OR c.parent_comment_id IN (SELECT id FROM tops)
ORDER BY c.created_at, c.id;

-- name: GetComment :one
SELECT id FROM feed_comments WHERE id = sqlc.arg(id);
