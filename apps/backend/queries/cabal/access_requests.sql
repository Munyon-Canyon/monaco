-- name: InsertAccessRequest :execrows
INSERT INTO cabal_access_requests (id, cabal_id, user_id, direction, invited_by, expires_at, created_at)
VALUES (sqlc.arg(id), sqlc.arg(cabal_id), sqlc.arg(user_id), sqlc.arg(direction), sqlc.narg(invited_by),
  sqlc.narg(expires_at), sqlc.arg(now))
ON CONFLICT (cabal_id, user_id) WHERE status = 'pending' DO NOTHING;

-- name: InsertInvite :execrows
INSERT INTO cabal_access_requests (id, cabal_id, user_id, direction, invited_by, expires_at, created_at)
VALUES (sqlc.arg(id), sqlc.arg(cabal_id), sqlc.arg(user_id), 'invite', sqlc.arg(invited_by)::uuid,
  sqlc.arg(expires_at)::timestamptz, sqlc.arg(now))
ON CONFLICT (cabal_id, user_id) WHERE status = 'pending' DO NOTHING;

-- name: FindAccessRequest :one
SELECT id, cabal_id, user_id, direction, invited_by, status, expires_at, decided_by, created_at, decided_at
FROM cabal_access_requests
WHERE cabal_id = $1 AND id = $2;

-- name: FindRequesterAccess :one
SELECT r.id, r.cabal_id, r.user_id, r.direction, r.invited_by, r.status, r.expires_at, r.decided_by, r.created_at,
  r.decided_at
FROM cabal_access_requests r
WHERE r.cabal_id = $1 AND r.user_id = $2 AND (
  r.status = 'pending' OR (r.status = 'denied' AND r.direction = 'request' AND NOT EXISTS (
    SELECT 1 FROM cabal_access_requests n
    WHERE n.cabal_id = r.cabal_id AND n.user_id = r.user_id AND n.direction = 'request'
      AND (n.created_at, n.id) > (r.created_at, r.id)
  ))
)
ORDER BY (r.status = 'pending') DESC, r.created_at DESC, r.id DESC
LIMIT 1;

-- name: DecideAccessRequest :execrows
UPDATE cabal_access_requests SET status = sqlc.arg(status), decided_by = sqlc.arg(decided_by)::uuid,
  decided_at = sqlc.arg(now)::timestamptz
WHERE cabal_id = sqlc.arg(cabal_id) AND id = sqlc.arg(id) AND status = 'pending';

-- name: ExpireAccessRequest :execrows
UPDATE cabal_access_requests SET status = 'expired', decided_at = sqlc.arg(now)::timestamptz
WHERE cabal_id = sqlc.arg(cabal_id) AND id = sqlc.arg(id) AND status = 'pending' AND direction = 'invite';

-- name: ListDueInvites :many
SELECT id, cabal_id, user_id, invited_by, expires_at FROM cabal_access_requests
WHERE status = 'pending' AND direction = 'invite' AND expires_at < sqlc.arg(now)::timestamptz
ORDER BY expires_at, id
LIMIT sqlc.arg(max_rows);

-- name: ListPendingRequestsForCabal :many
SELECT id, user_id, created_at FROM cabal_access_requests
WHERE cabal_id = $1 AND direction = 'request' AND status = 'pending'
ORDER BY created_at, id;

-- name: ListPendingInvitesForCabal :many
SELECT id, user_id, invited_by, expires_at, created_at FROM cabal_access_requests
WHERE cabal_id = sqlc.arg(cabal_id) AND direction = 'invite' AND status = 'pending'
  AND expires_at >= sqlc.arg(now)::timestamptz
ORDER BY created_at, id;

-- name: ListPendingInvitesForUser :many
SELECT r.id, r.cabal_id, r.invited_by, r.expires_at, r.created_at, c.name AS cabal_name,
  c.picture_url AS cabal_picture_url,
  (SELECT count(*) FROM cabal_members m WHERE m.cabal_id = c.id)::int AS member_count
FROM cabal_access_requests r
JOIN cabals c ON c.id = r.cabal_id
WHERE r.user_id = sqlc.arg(user_id) AND r.direction = 'invite' AND r.status = 'pending'
  AND r.expires_at >= sqlc.arg(now)::timestamptz AND c.status = 'active'
ORDER BY r.created_at, r.id;
