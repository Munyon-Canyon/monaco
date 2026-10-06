-- name: InsertContactMatches :exec
INSERT INTO contact_matches (user_id, matched_user_id, source, created_at)
SELECT sqlc.arg(user_id), matched, 'phone', sqlc.arg(created_at)
FROM unnest(sqlc.arg(matched_user_ids)::uuid[]) AS matched
ON CONFLICT (user_id, matched_user_id, source) DO NOTHING;

-- name: ListContactMatches :many
SELECT cm.matched_user_id, cm.created_at,
  EXISTS (
    SELECT 1 FROM follows f
    WHERE f.follower_id = sqlc.arg(user_id)
      AND f.followee_id = cm.matched_user_id
      AND f.deleted_at IS NULL
  ) AS followed_by_me
FROM contact_matches cm
WHERE cm.user_id = sqlc.arg(user_id)
  AND cm.dismissed_at IS NULL
  AND (
    NOT sqlc.arg(has_cursor)::bool
    OR (cm.created_at, cm.matched_user_id) < (sqlc.arg(after_at)::timestamptz, sqlc.arg(after_id)::uuid)
  )
ORDER BY cm.created_at DESC, cm.matched_user_id DESC
LIMIT sqlc.arg(row_limit)::int;
