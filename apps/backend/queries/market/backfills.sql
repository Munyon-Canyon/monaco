-- name: InsertPendingBackfills :execrows
INSERT INTO price_backfills (mint, requested_at)
SELECT u.mint, sqlc.arg(now)::timestamptz
FROM unnest(sqlc.arg(mints)::text[]) AS u (mint)
ON CONFLICT (mint) DO NOTHING;

-- name: PendingBackfills :many
SELECT mint
FROM price_backfills
WHERE done_at IS NULL
  AND (
    last_attempt_at IS NULL
    OR last_attempt_at + LEAST(interval '5 minutes' * power(2, LEAST(attempts, 12)), interval '24 hours')
      <= sqlc.arg(now)::timestamptz
  )
ORDER BY last_code IS NOT NULL, requested_at, mint
LIMIT sqlc.arg(batch_limit)::integer;

-- name: FinishBackfill :exec
UPDATE price_backfills
SET done_at = sqlc.arg(now)::timestamptz, last_code = NULL, attempts = 0, last_attempt_at = NULL
WHERE mint = sqlc.arg(mint)::text;

-- name: FailBackfill :exec
UPDATE price_backfills
SET last_code = sqlc.arg(code)::text,
    attempts = attempts + sqlc.arg(back_off)::boolean::integer,
    last_attempt_at = CASE WHEN sqlc.arg(back_off)::boolean THEN sqlc.arg(now)::timestamptz ELSE last_attempt_at END
WHERE mint = sqlc.arg(mint)::text;

-- name: RequestBackfills :execrows
INSERT INTO price_backfills (mint, requested_at)
SELECT u.mint, sqlc.arg(now)::timestamptz
FROM unnest(sqlc.arg(mints)::text[]) AS u (mint)
ON CONFLICT (mint) DO UPDATE SET requested_at = excluded.requested_at, done_at = NULL, last_code = NULL, attempts = 0, last_attempt_at = NULL;
