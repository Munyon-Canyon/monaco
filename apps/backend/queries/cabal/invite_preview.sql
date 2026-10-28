-- name: InvitePreview :one
SELECT c.id, c.name, c.picture_url, c.join_mode, c.status, c.created_at,
  (SELECT count(*) FROM cabal_members m WHERE m.cabal_id = c.id)::int AS member_count,
  EXISTS (
    SELECT 1 FROM cabal_members m WHERE m.cabal_id = c.id AND m.user_id = sqlc.arg(inviter)
  ) AS inviter_is_member,
  ARRAY(
    SELECT m.user_id FROM cabal_members m
    WHERE m.cabal_id = c.id AND m.user_id <> sqlc.arg(inviter)
    ORDER BY m.joined_at, m.user_id
    LIMIT 4
  )::uuid[] AS first_member_ids
FROM cabals c
WHERE c.id = sqlc.arg(cabal_id);
