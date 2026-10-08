-- name: InsertCabal :execrows
INSERT INTO cabals (id, name, creator_id, join_mode, voter_mode, threshold, proposal_expiry_seconds, slippage_bps,
  created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(creator_id), sqlc.arg(join_mode), sqlc.arg(voter_mode),
  sqlc.arg(threshold), sqlc.arg(proposal_expiry_seconds), sqlc.arg(slippage_bps),
  sqlc.arg(now), sqlc.arg(now));

-- name: LockCabalShared :one
SELECT id FROM cabals WHERE id = $1 FOR SHARE;

-- name: LockCabalExclusive :one
SELECT id FROM cabals WHERE id = $1 FOR NO KEY UPDATE;

-- name: LockCabalForAccess :one
SELECT c.creator_id, c.join_mode, c.voter_mode, c.threshold, c.proposal_expiry_seconds, c.slippage_bps, c.status,
  ARRAY(
    SELECT m.user_id FROM cabal_members m WHERE m.cabal_id = c.id AND m.user_id = ANY(sqlc.arg(user_ids)::uuid[])
  )::uuid[] AS member_ids
FROM cabals c
WHERE c.id = sqlc.arg(cabal_id)
FOR SHARE OF c;

-- name: FindCabal :one
SELECT c.id, c.name, c.picture_url, c.creator_id, c.join_mode, c.voter_mode, c.threshold,
  c.proposal_expiry_seconds, c.slippage_bps, c.status, c.created_at, c.updated_at,
  (SELECT count(*) FROM cabal_members m WHERE m.cabal_id = c.id)::int AS member_count
FROM cabals c
WHERE c.id = $1;

-- name: ListCabals :many
SELECT c.id, c.name, c.picture_url, c.creator_id, c.join_mode, c.voter_mode, c.threshold,
  c.proposal_expiry_seconds, c.slippage_bps, c.status, c.created_at, c.updated_at,
  (SELECT count(*) FROM cabal_members m WHERE m.cabal_id = c.id)::int AS member_count
FROM cabals c
WHERE c.id = ANY(sqlc.arg(cabal_ids)::uuid[])
ORDER BY c.created_at, c.id;

-- name: AllCabals :many
SELECT c.id, c.name, c.picture_url, c.creator_id, c.join_mode, c.voter_mode, c.threshold,
  c.proposal_expiry_seconds, c.slippage_bps, c.status, c.created_at, c.updated_at,
  (SELECT count(*) FROM cabal_members m WHERE m.cabal_id = c.id)::int AS member_count
FROM cabals c
ORDER BY c.created_at, c.id;

-- name: SearchCabals :many
SELECT c.id, c.name, c.picture_url, c.join_mode, c.created_at,
  (SELECT count(*) FROM cabal_members m WHERE m.cabal_id = c.id)::int AS member_count,
  EXISTS (SELECT 1 FROM cabal_members m WHERE m.cabal_id = c.id AND m.user_id = sqlc.arg(actor_id)) AS is_member,
  r.status AS my_access_request_status
FROM cabals c
LEFT JOIN cabal_access_requests r ON r.cabal_id = c.id AND r.user_id = sqlc.arg(actor_id)
  AND r.direction = 'request' AND r.status = 'pending'
WHERE c.status <> 'banned'
  AND (sqlc.arg(query)::text = '' OR lower(c.name) LIKE '%' || lower(sqlc.arg(query)::text) || '%')
  AND (SELECT count(*) FROM cabal_members m WHERE m.cabal_id = c.id) > 0
  AND (
    sqlc.narg(cursor_member_count)::int IS NULL
    OR (SELECT count(*) FROM cabal_members m WHERE m.cabal_id = c.id)::int < sqlc.narg(cursor_member_count)::int
    OR (
      (SELECT count(*) FROM cabal_members m WHERE m.cabal_id = c.id)::int = sqlc.narg(cursor_member_count)::int
      AND (c.created_at, c.id) < (sqlc.narg(cursor_created_at)::timestamptz, sqlc.narg(cursor_id)::uuid)
    )
  )
ORDER BY member_count DESC, c.created_at DESC, c.id DESC
LIMIT sqlc.arg(page_size)::int;

-- name: ListMyCabals :many
SELECT c.id, c.name, c.picture_url, m.role, m.can_vote, m.joined_at,
  (SELECT count(*) FROM cabal_members cm WHERE cm.cabal_id = c.id)::int AS member_count,
  CASE WHEN m.role = 'creator' AND c.join_mode = 'request' THEN
    (SELECT count(*) FROM cabal_access_requests ar
      WHERE ar.cabal_id = c.id AND ar.direction = 'request' AND ar.status = 'pending')::int
  ELSE 0 END AS pending_request_count
FROM cabal_members m
JOIN cabals c ON c.id = m.cabal_id
WHERE m.user_id = sqlc.arg(user_id)
ORDER BY m.joined_at DESC, c.id DESC;

-- name: UpdateCabal :execrows
UPDATE cabals SET name = sqlc.arg(name), join_mode = sqlc.arg(join_mode), voter_mode = sqlc.arg(voter_mode),
  threshold = sqlc.arg(threshold), proposal_expiry_seconds = sqlc.arg(proposal_expiry_seconds),
  slippage_bps = sqlc.arg(slippage_bps), updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id);

-- name: SetCabalPicture :execrows
UPDATE cabals SET picture_url = NULLIF(sqlc.arg(picture_url)::text, ''), updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id);

-- name: CabalCounts :one
WITH sizes AS (
  SELECT c.status, count(m.user_id) AS members
  FROM cabals c
  LEFT JOIN cabal_members m ON m.cabal_id = c.id
  GROUP BY c.id, c.status
)
SELECT count(*)::bigint AS cabals,
  (count(*) FILTER (WHERE status = 'banned'))::bigint AS banned,
  coalesce(percentile_disc(0.5) WITHIN GROUP (ORDER BY members), 0)::bigint AS members_p50,
  coalesce(percentile_disc(0.9) WITHIN GROUP (ORDER BY members), 0)::bigint AS members_p90,
  coalesce(max(members), 0)::bigint AS members_max
FROM sizes;
