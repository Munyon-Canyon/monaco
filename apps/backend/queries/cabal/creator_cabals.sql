-- name: LockCreatedCabalMemberCounts :many
SELECT c.id, (SELECT count(*) FROM cabal_members m WHERE m.cabal_id = c.id)::bigint AS members
FROM cabals c
WHERE c.creator_id = $1
ORDER BY c.id
FOR UPDATE OF c;
