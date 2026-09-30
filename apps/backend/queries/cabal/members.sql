-- name: InsertMember :execrows
INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (cabal_id, user_id) DO NOTHING;

-- name: FindMember :one
SELECT role, can_vote, joined_at FROM cabal_members
WHERE cabal_id = $1 AND user_id = $2;

-- name: ListMembers :many
SELECT user_id, role, can_vote, joined_at FROM cabal_members
WHERE cabal_id = $1
ORDER BY joined_at, user_id;

-- name: ListVoterIDs :many
SELECT user_id FROM cabal_members
WHERE cabal_id = $1 AND can_vote
ORDER BY joined_at, user_id;

-- name: ListCabalIDsForUser :many
SELECT cabal_id FROM cabal_members
WHERE user_id = $1
ORDER BY joined_at, cabal_id;

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
UPDATE cabal_members SET can_vote = (role = 'creator' OR user_id = ANY(coalesce(sqlc.arg(voter_ids)::uuid[], '{}')))
WHERE cabal_id = sqlc.arg(cabal_id);

-- name: SetAllMembersVote :exec
UPDATE cabal_members SET can_vote = true
WHERE cabal_id = $1;
