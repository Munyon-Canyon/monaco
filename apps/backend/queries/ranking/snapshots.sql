-- name: InsertCabalValueSnapshot :exec
INSERT INTO cabal_value_snapshots (cabal_id, at, value_micros, nav_per_share_micros, total_shares)
VALUES (sqlc.arg(cabal_id), sqlc.arg(at), sqlc.arg(value_micros), sqlc.arg(nav_per_share_micros), sqlc.arg(total_shares));

-- name: LatestCabalValues :many
SELECT DISTINCT ON (cabal_id) cabal_id, at, value_micros, nav_per_share_micros, total_shares
FROM cabal_value_snapshots
ORDER BY cabal_id, at DESC;
