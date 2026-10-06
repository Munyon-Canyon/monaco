-- name: SnapshotsSince :many
SELECT s.at, s.value_micros, s.nav_per_share_micros, s.total_shares
FROM cabal_value_snapshots AS s
WHERE s.cabal_id = sqlc.arg(cabal_id)
  AND s.at <= sqlc.arg(until)::timestamptz
  AND s.at >= coalesce(
    (SELECT max(prior.at) FROM cabal_value_snapshots AS prior
     WHERE prior.cabal_id = sqlc.arg(cabal_id) AND prior.at <= sqlc.arg(since)::timestamptz),
    sqlc.arg(since)::timestamptz)
ORDER BY s.at;

-- name: SnapshotsOfCabals :many
SELECT s.cabal_id, s.at, s.value_micros, s.nav_per_share_micros, s.total_shares
FROM cabal_value_snapshots AS s
WHERE s.cabal_id = ANY(sqlc.arg(cabal_ids)::uuid[])
  AND s.at <= sqlc.arg(until)::timestamptz
  AND s.at >= coalesce(
    (SELECT max(prior.at) FROM cabal_value_snapshots AS prior
     WHERE prior.cabal_id = s.cabal_id AND prior.at <= sqlc.arg(since)::timestamptz),
    sqlc.arg(since)::timestamptz)
ORDER BY s.cabal_id, s.at;

-- name: LatestValuesOfCabals :many
SELECT DISTINCT ON (cabal_id) cabal_id, at, value_micros, nav_per_share_micros, total_shares
FROM cabal_value_snapshots
WHERE cabal_id = ANY(sqlc.arg(cabal_ids)::uuid[])
ORDER BY cabal_id, at DESC;
