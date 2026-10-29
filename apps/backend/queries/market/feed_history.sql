-- name: WindowCloses :many
WITH w AS (
  SELECT u.key, a.mint, u.from_ts, u.to_ts
  FROM ROWS FROM (
    unnest(sqlc.arg(keys)::uuid[]),
    unnest(sqlc.arg(asset_ids)::uuid[]),
    unnest(sqlc.arg(from_ts)::timestamptz[]),
    unnest(sqlc.arg(to_ts)::timestamptz[])
  ) AS u (key, asset_id, from_ts, to_ts)
  JOIN assets AS a ON a.id = u.asset_id
  WHERE u.from_ts < u.to_ts
),
binned AS (
  SELECT w.key, p.ts, p.price_micros,
    least(
      width_bucket(
        extract(epoch FROM p.ts)::float8,
        extract(epoch FROM w.from_ts)::float8,
        extract(epoch FROM w.to_ts)::float8,
        sqlc.arg(buckets)::integer
      ),
      sqlc.arg(buckets)::integer
    ) AS bucket
  FROM w
  JOIN price_points AS p ON p.mint = w.mint AND p.ts >= w.from_ts AND p.ts <= w.to_ts
)
SELECT DISTINCT ON (binned.key, binned.bucket) binned.key::uuid AS window_key, binned.ts, binned.price_micros
FROM binned
ORDER BY binned.key, binned.bucket, binned.ts DESC;
