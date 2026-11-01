-- name: DeleteMembershipsOf :many
DELETE FROM cabal_members
WHERE user_id = $1
RETURNING cabal_id, can_vote;

-- name: SuspendMembershipsOf :many
UPDATE cabal_members SET suspended = true, role = 'member', can_vote = false
WHERE user_id = $1 AND NOT suspended
RETURNING cabal_id;

-- name: RestoreMembershipsOf :many
UPDATE cabal_members m SET suspended = false, can_vote = (c.voter_mode = 'all')
FROM cabals c
WHERE m.cabal_id = c.id AND m.user_id = $1 AND m.suspended
RETURNING m.cabal_id;

-- name: PromoteLongestStandingMember :one
UPDATE cabal_members SET role = 'admin', can_vote = true
WHERE (cabal_id, user_id) = (
  SELECT m.cabal_id, m.user_id FROM cabal_members m
  WHERE m.cabal_id = sqlc.arg(cabal_id) AND NOT m.suspended
    AND NOT EXISTS (
      SELECT 1 FROM cabal_members a WHERE a.cabal_id = m.cabal_id AND a.role IN ('creator', 'admin')
    )
  ORDER BY m.joined_at, m.user_id
  LIMIT 1
)
RETURNING user_id;
