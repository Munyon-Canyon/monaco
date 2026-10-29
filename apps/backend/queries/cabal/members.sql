-- name: InsertMember :execrows
INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (cabal_id, user_id) DO NOTHING;

-- name: FindMember :one
SELECT role, can_vote, joined_at FROM cabal_members
WHERE cabal_id = $1 AND user_id = $2;

-- name: LockMember :one
SELECT 1 FROM cabal_members
WHERE cabal_id = $1 AND user_id = $2
FOR KEY SHARE;

-- name: ListMembers :many
SELECT user_id, role, can_vote, joined_at FROM cabal_members
WHERE cabal_id = $1
ORDER BY joined_at, user_id;

-- name: ListMembersPage :many
SELECT user_id, role, can_vote, joined_at FROM cabal_members
WHERE cabal_id = @cabal_id
  AND (NOT @has_cursor::boolean OR (joined_at, user_id) > (@cursor_joined_at::timestamptz, @cursor_user_id::uuid))
ORDER BY joined_at, user_id
LIMIT @row_limit;

-- name: ListMembersOf :many
SELECT cabal_id, user_id, role, can_vote, joined_at FROM cabal_members
WHERE cabal_id = ANY(sqlc.arg(cabal_ids)::uuid[])
ORDER BY cabal_id, joined_at, user_id;

-- name: ListVoterIDs :many
SELECT user_id FROM cabal_members
WHERE cabal_id = $1 AND can_vote
ORDER BY joined_at, user_id;

-- name: ListAdminIDs :many
SELECT user_id FROM cabal_members
WHERE cabal_id = $1 AND role IN ('creator', 'admin')
ORDER BY (role = 'creator') DESC, joined_at, user_id;

-- name: ListCabalIDsForUser :many
SELECT cabal_id FROM cabal_members
WHERE user_id = $1
ORDER BY sort_index NULLS LAST, joined_at, cabal_id;

-- name: LockMembers :many
SELECT user_id, role FROM cabal_members
WHERE cabal_id = $1
ORDER BY user_id
FOR UPDATE;

-- name: DeleteMember :one
DELETE FROM cabal_members
WHERE cabal_id = $1 AND user_id = $2
RETURNING role, can_vote;

-- name: SetMemberVoters :exec
UPDATE cabal_members SET can_vote = (
  role IN ('creator', 'admin') OR user_id = ANY(coalesce(sqlc.arg(voter_ids)::uuid[], '{}'))
)
WHERE cabal_id = sqlc.arg(cabal_id);

-- name: SetAllMembersVote :exec
UPDATE cabal_members SET can_vote = true
WHERE cabal_id = $1;

-- name: LockMyMemberships :many
SELECT cabal_id FROM cabal_members
WHERE user_id = $1
ORDER BY sort_index NULLS LAST, joined_at, cabal_id
FOR UPDATE;

-- name: SetMySortIndexes :exec
UPDATE cabal_members m SET sort_index = v.sort_index
FROM (
  SELECT unnest(sqlc.arg(cabal_ids)::uuid[]) AS cabal_id, unnest(sqlc.arg(sort_indexes)::int[]) AS sort_index
) v
WHERE m.user_id = sqlc.arg(user_id) AND m.cabal_id = v.cabal_id;
