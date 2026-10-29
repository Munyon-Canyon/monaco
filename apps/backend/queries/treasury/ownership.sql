-- name: MemberSharesInCabalsOf :many
SELECT cabal_id, user_id, share_units::text AS share_units
FROM user_positions
WHERE cabal_id IN (
  SELECT held.cabal_id FROM user_positions AS held
  WHERE held.user_id = sqlc.arg(user_id)::uuid AND held.share_units > 0
)
ORDER BY cabal_id, user_id;

-- name: ShareUnitsByCabal :many
SELECT user_id, share_units::text AS share_units
FROM user_positions
WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND share_units > 0
ORDER BY user_id;
