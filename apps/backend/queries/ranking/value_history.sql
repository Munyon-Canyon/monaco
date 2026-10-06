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
