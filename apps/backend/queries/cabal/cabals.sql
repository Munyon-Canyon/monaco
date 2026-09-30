-- name: InsertCabal :execrows
INSERT INTO cabals (id, name, creator_id, join_mode, voter_mode, threshold, proposal_expiry_seconds, slippage_bps,
  invite_code, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(creator_id), sqlc.arg(join_mode), sqlc.arg(voter_mode),
  sqlc.arg(threshold), sqlc.arg(proposal_expiry_seconds), sqlc.arg(slippage_bps), sqlc.arg(invite_code),
  sqlc.arg(now), sqlc.arg(now))
ON CONFLICT (invite_code) DO NOTHING;

-- name: LockCabalShared :one
SELECT id FROM cabals WHERE id = $1 FOR SHARE;

-- name: LockCabalExclusive :one
SELECT id FROM cabals WHERE id = $1 FOR UPDATE;

-- name: FindCabal :one
SELECT c.id, c.name, c.picture_url, c.creator_id, c.join_mode, c.voter_mode, c.threshold,
  c.proposal_expiry_seconds, c.slippage_bps, c.invite_code, c.status, c.created_at, c.updated_at,
  (SELECT count(*) FROM cabal_members m WHERE m.cabal_id = c.id)::int AS member_count
FROM cabals c
WHERE c.id = $1;

-- name: FindCabalByInviteCode :one
SELECT c.id, c.name, c.picture_url, c.creator_id, c.join_mode, c.voter_mode, c.threshold,
  c.proposal_expiry_seconds, c.slippage_bps, c.invite_code, c.status, c.created_at, c.updated_at,
  (SELECT count(*) FROM cabal_members m WHERE m.cabal_id = c.id)::int AS member_count
FROM cabals c
WHERE c.invite_code = $1;

-- name: ListCabals :many
SELECT c.id, c.name, c.picture_url, c.creator_id, c.join_mode, c.voter_mode, c.threshold,
  c.proposal_expiry_seconds, c.slippage_bps, c.invite_code, c.status, c.created_at, c.updated_at,
  (SELECT count(*) FROM cabal_members m WHERE m.cabal_id = c.id)::int AS member_count
FROM cabals c
WHERE c.id = ANY(sqlc.arg(cabal_ids)::uuid[])
ORDER BY c.created_at, c.id;

-- name: UpdateCabal :execrows
UPDATE cabals SET name = sqlc.arg(name), join_mode = sqlc.arg(join_mode), voter_mode = sqlc.arg(voter_mode),
  threshold = sqlc.arg(threshold), proposal_expiry_seconds = sqlc.arg(proposal_expiry_seconds),
  slippage_bps = sqlc.arg(slippage_bps), updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id);

-- name: SetCabalPicture :execrows
UPDATE cabals SET picture_url = sqlc.narg(picture_url), updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id);
