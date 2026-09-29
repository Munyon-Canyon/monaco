-- name: TakeRateLimitTokens :one
INSERT INTO rate_limit_buckets AS b (key, tokens_milli, updated_at)
VALUES (sqlc.arg(key), sqlc.arg(burst_milli)::bigint - sqlc.arg(cost_milli)::bigint, sqlc.arg(now)::timestamptz)
ON CONFLICT (key) DO UPDATE
SET tokens_milli = least(
    sqlc.arg(burst_milli)::bigint,
    b.tokens_milli + div(
      greatest(extract(epoch FROM sqlc.arg(now)::timestamptz - b.updated_at), 0) * 1000000
        * sqlc.arg(rate_milli)::bigint,
      sqlc.arg(per_micros)::bigint
    )
  )::bigint - sqlc.arg(cost_milli)::bigint,
  updated_at = greatest(b.updated_at, sqlc.arg(now)::timestamptz)
WHERE least(
    sqlc.arg(burst_milli)::bigint,
    b.tokens_milli + div(
      greatest(extract(epoch FROM sqlc.arg(now)::timestamptz - b.updated_at), 0) * 1000000
        * sqlc.arg(rate_milli)::bigint,
      sqlc.arg(per_micros)::bigint
    )
  ) >= sqlc.arg(cost_milli)::bigint
RETURNING b.tokens_milli;

-- name: GetRateLimitBucket :one
SELECT tokens_milli, updated_at FROM rate_limit_buckets WHERE key = $1;

-- name: DeleteRateLimitBucketsIdleBefore :execrows
DELETE FROM rate_limit_buckets WHERE updated_at < $1;
