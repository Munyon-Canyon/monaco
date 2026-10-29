-- name: UserReplies :many
SELECT c.id, COALESCE(c.body, 'GIF')::text AS body, c.created_at, o.ref_id AS proposal_id
FROM feed_comments AS c
JOIN feed_objects AS o ON o.id = c.feed_object_id
WHERE c.author_id = @author_id
  AND c.deleted_at IS NULL
  AND o.kind = 'proposal' AND o.ref_type = 'proposals'
  AND (NOT @has_cursor::bool OR (c.created_at, c.id) < (@cursor_at::timestamptz, @cursor_id::uuid))
ORDER BY c.created_at DESC, c.id DESC
LIMIT @row_limit;
