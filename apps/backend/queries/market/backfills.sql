-- name: InsertPendingBackfills :execrows
INSERT INTO price_backfills (mint, requested_at)
SELECT u.mint, sqlc.arg(now)::timestamptz
FROM unnest(sqlc.arg(mints)::text[]) AS u (mint)
ON CONFLICT (mint) DO NOTHING;

-- name: PendingBackfills :many
SELECT mint
FROM price_backfills
WHERE done_at IS NULL
ORDER BY last_code IS NOT NULL, requested_at, mint
LIMIT sqlc.arg(batch_limit)::integer;

-- name: FinishBackfill :exec
UPDATE price_backfills SET done_at = sqlc.arg(now)::timestamptz, last_code = NULL
WHERE mint = sqlc.arg(mint)::text;

-- name: FailBackfill :exec
UPDATE price_backfills SET last_code = sqlc.arg(code)::text
WHERE mint = sqlc.arg(mint)::text;

-- name: RequestBackfills :execrows
INSERT INTO price_backfills (mint, requested_at)
SELECT u.mint, sqlc.arg(now)::timestamptz
FROM unnest(sqlc.arg(mints)::text[]) AS u (mint)
ON CONFLICT (mint) DO UPDATE SET requested_at = excluded.requested_at, done_at = NULL, last_code = NULL;
