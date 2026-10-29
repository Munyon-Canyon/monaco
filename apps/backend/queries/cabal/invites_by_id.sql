-- name: InvitesByID :many
SELECT r.id, r.cabal_id, r.invited_by, r.expires_at, r.created_at, c.name AS cabal_name,
  c.picture_url AS cabal_picture_url,
  (SELECT count(*) FROM cabal_members m WHERE m.cabal_id = c.id)::int AS member_count
FROM cabal_access_requests r
JOIN cabals c ON c.id = r.cabal_id
WHERE r.id = ANY(sqlc.arg(request_ids)::uuid[]) AND r.direction = 'invite'
ORDER BY r.created_at, r.id;
