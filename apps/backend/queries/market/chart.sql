-- name: EarliestPrice :one
SELECT ts
FROM price_points
WHERE mint = sqlc.arg(mint)::text
ORDER BY ts ASC
LIMIT 1;

-- name: ChartBucketsPage :many
SELECT newest.bucket, newest.open_micros, newest.high_micros, newest.low_micros, newest.close_micros
FROM (
  SELECT date_bin(
      (sqlc.arg(bucket_seconds)::bigint * interval '1 second'),
      ts,
      CASE WHEN sqlc.arg(bucket_seconds)::bigint = 604800
        THEN timestamptz '2000-01-03 00:00:00+00'
        ELSE timestamptz '2000-01-01 00:00:00+00'
      END
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
  ORDER BY bucket DESC
  LIMIT sqlc.arg(page_limit)::integer
) newest
ORDER BY newest.bucket ASC;

-- name: HasSampleBefore :one
SELECT EXISTS (
  SELECT 1
  FROM price_points
  WHERE mint = sqlc.arg(mint)::text
    AND ts < sqlc.arg(before)::timestamptz
);
