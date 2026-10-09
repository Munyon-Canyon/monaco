-- name: InsertPendingBackfills :execrows
INSERT INTO price_backfills (mint, requested_at)
SELECT a.mint, sqlc.arg(now)::timestamptz
FROM assets AS a
WHERE a.mint = ANY (sqlc.arg(mints)::text[])
  AND a.chain_checked_at IS NOT NULL AND coalesce(a.tradable_override, a.issuer_tradable)
ON CONFLICT (mint) DO UPDATE
SET requested_at = excluded.requested_at, done_at = NULL, last_code = NULL, attempts = 0, last_attempt_at = NULL
WHERE price_backfills.last_code = 'coingecko_not_listed'
  AND price_backfills.done_at <= excluded.requested_at - interval '7 days';

-- name: PendingBackfills :many
SELECT b.mint
FROM price_backfills AS b
JOIN assets AS a ON a.mint = b.mint
WHERE b.done_at IS NULL
  AND a.chain_checked_at IS NOT NULL AND coalesce(a.tradable_override, a.issuer_tradable)
  AND (
    b.last_attempt_at IS NULL
    OR b.last_attempt_at + LEAST(interval '5 minutes' * power(2, LEAST(b.attempts, 12)), interval '24 hours')
      <= sqlc.arg(now)::timestamptz
  )
ORDER BY b.last_attempt_at NULLS FIRST, a.popular_rank NULLS LAST, b.requested_at, b.mint
LIMIT sqlc.arg(batch_limit)::integer;

-- name: FinishBackfill :exec
UPDATE price_backfills
SET done_at = sqlc.arg(now)::timestamptz, last_code = NULL, attempts = 0, last_attempt_at = NULL
WHERE mint = sqlc.arg(mint)::text;

-- name: MarkBackfillUnlisted :exec
UPDATE price_backfills
SET done_at = sqlc.arg(now)::timestamptz, last_code = 'coingecko_not_listed', attempts = 0, last_attempt_at = NULL
WHERE mint = sqlc.arg(mint)::text;

-- name: FailBackfill :exec
UPDATE price_backfills
SET last_code = sqlc.arg(code)::text,
    attempts = attempts + 1,
    last_attempt_at = sqlc.arg(now)::timestamptz
WHERE mint = sqlc.arg(mint)::text;

-- name: RequestBackfills :execrows
INSERT INTO price_backfills (mint, requested_at)
SELECT u.mint, sqlc.arg(now)::timestamptz
FROM unnest(sqlc.arg(mints)::text[]) AS u (mint)
ON CONFLICT (mint) DO UPDATE SET requested_at = excluded.requested_at, done_at = NULL, last_code = NULL, attempts = 0, last_attempt_at = NULL;

-- name: QueueNewListingBackfills :execrows
INSERT INTO price_backfills (mint, requested_at)
SELECT a.mint, sqlc.arg(now)::timestamptz
FROM assets AS a
WHERE a.mint = ANY (sqlc.arg(mints)::text[])
  AND a.first_seen_at = sqlc.arg(now)::timestamptz
  AND coalesce(a.tradable_override, a.issuer_tradable)
ON CONFLICT (mint) DO NOTHING;
