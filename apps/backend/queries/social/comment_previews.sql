-- name: CommentPreviews :many
WITH RECURSIVE requested AS (
  SELECT c.id, c.reply_to_comment_id
  FROM feed_comments c
  WHERE c.id = ANY(sqlc.arg(comment_ids)::uuid[])
), wanted AS (
  SELECT id FROM requested
  UNION
  SELECT reply_to_comment_id FROM requested WHERE reply_to_comment_id IS NOT NULL
), chain AS (
  SELECT c.id AS start_id, c.id, c.parent_comment_id
  FROM feed_comments c
  WHERE c.id IN (SELECT id FROM wanted)
  UNION ALL
  SELECT ch.start_id, p.id, p.parent_comment_id
  FROM chain ch
  JOIN feed_comments p ON p.id = ch.parent_comment_id
), roots AS (
  SELECT start_id, id AS root_comment_id FROM chain WHERE parent_comment_id IS NULL
)
SELECT c.id, c.feed_object_id, c.parent_comment_id, c.reply_to_comment_id, r.root_comment_id, c.author_id, c.body, c.created_at,
  (o.kind = 'proposal')::bool AS on_proposal, o.ref_id, o.cabal_id
FROM feed_comments c
JOIN roots r ON r.start_id = c.id
JOIN feed_objects o ON o.id = c.feed_object_id
WHERE c.id IN (SELECT id FROM wanted)
  AND c.deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM user_blocks b WHERE b.blocker_id = sqlc.arg(viewer)::uuid AND b.blocked_id = c.author_id
  )
ORDER BY c.created_at, c.id;
