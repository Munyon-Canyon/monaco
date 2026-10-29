-- name: TopHoldings :many
SELECT DISTINCT ON (cabal_id) cabal_id, asset
FROM cabal_positions
WHERE cabal_id = ANY(@cabal_ids::uuid[]) AND units > 0
ORDER BY cabal_id, cost_basis_micros DESC, asset;
