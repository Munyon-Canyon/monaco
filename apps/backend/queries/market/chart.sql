-- name: EarliestPrice :one
SELECT ts
FROM price_points
WHERE mint = sqlc.arg(mint)::text
ORDER BY ts ASC
LIMIT 1;

-- name: ChartBuckets :many
SELECT date_bin(
    (sqlc.arg(bucket_seconds)::bigint * interval '1 second'),
    ts,
    timestamptz '2000-01-01 00:00:00+00'
  )::timestamptz AS bucket,
  (array_agg(price_micros ORDER BY ts ASC))[1]::bigint AS open_micros,
  max(price_micros)::bigint AS high_micros,
  min(price_micros)::bigint AS low_micros,
  (array_agg(price_micros ORDER BY ts DESC))[1]::bigint AS close_micros
FROM price_points
WHERE mint = sqlc.arg(mint)::text
  AND ts >= sqlc.arg(since)::timestamptz
  AND ts <= sqlc.arg(until)::timestamptz
GROUP BY bucket
ORDER BY bucket;
