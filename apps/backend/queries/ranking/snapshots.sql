-- name: InsertCabalValueSnapshots :exec
INSERT INTO cabal_value_snapshots (cabal_id, at, value_micros, nav_per_share_micros, total_shares)
SELECT cabal_id, at, value_micros, nav_per_share_micros, total_shares
FROM jsonb_to_recordset(sqlc.arg(rows)::jsonb) AS rows(
  cabal_id uuid, at timestamptz, value_micros bigint, nav_per_share_micros bigint, total_shares bigint
);

-- name: LatestCabalValues :many
SELECT DISTINCT ON (cabal_id) cabal_id, at, value_micros, nav_per_share_micros, total_shares
FROM cabal_value_snapshots
ORDER BY cabal_id, at DESC;

-- name: SnapshotsAt :many
SELECT DISTINCT ON (snapshots.cabal_id, bucket.idx)
  (bucket.idx - 1)::int AS bucket, snapshots.cabal_id, snapshots.at, snapshots.value_micros,
  snapshots.nav_per_share_micros, snapshots.total_shares
FROM unnest(sqlc.arg(ts)::timestamptz[]) WITH ORDINALITY AS bucket(t, idx)
JOIN cabal_value_snapshots AS snapshots ON snapshots.at <= bucket.t
ORDER BY snapshots.cabal_id, bucket.idx, snapshots.at DESC;
