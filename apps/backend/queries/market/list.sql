-- name: ListAssetsBySymbol :many
SELECT id, symbol, mint, decimals, issuer, kind, display_name, logo_url, ui_multiplier_num, ui_multiplier_den, issuer_tradable, tradable_override, popular_rank, company_key, first_seen_at, updated_at, chain_checked_at, ui_multiplier_next_num, ui_multiplier_next_den, ui_multiplier_next_at FROM assets
WHERE chain_checked_at IS NOT NULL
  AND coalesce(tradable_override, issuer_tradable)
  AND (sqlc.arg(kind)::text = '' OR kind = sqlc.arg(kind)::text)
  AND (
    sqlc.arg(q)::text = ''
    OR symbol ILIKE sqlc.arg(prefix)::text ESCAPE '!'
    OR display_name ILIKE sqlc.arg(contains)::text ESCAPE '!'
  )
  AND (
    NOT sqlc.arg(has_cursor)::boolean
    OR (symbol, id) > (sqlc.arg(cursor_symbol)::text, sqlc.arg(cursor_id)::uuid)
  )
ORDER BY symbol, id
LIMIT sqlc.arg(row_limit)::integer;

-- name: ListAssetsByRank :many
SELECT id, symbol, mint, decimals, issuer, kind, display_name, logo_url, ui_multiplier_num, ui_multiplier_den, issuer_tradable, tradable_override, popular_rank, company_key, first_seen_at, updated_at, chain_checked_at, ui_multiplier_next_num, ui_multiplier_next_den, ui_multiplier_next_at FROM assets
WHERE chain_checked_at IS NOT NULL
  AND coalesce(tradable_override, issuer_tradable)
  AND popular_rank IS NOT NULL
  AND (
    sqlc.arg(q)::text = ''
    OR symbol ILIKE sqlc.arg(prefix)::text ESCAPE '!'
    OR display_name ILIKE sqlc.arg(contains)::text ESCAPE '!'
  )
  AND (
    NOT sqlc.arg(has_cursor)::boolean
    OR (popular_rank, id) > (sqlc.arg(cursor_rank)::smallint, sqlc.arg(cursor_id)::uuid)
  )
ORDER BY popular_rank, id
LIMIT sqlc.arg(row_limit)::integer;

-- name: NewestSamples :many
SELECT u.mint::text AS mint, p.ts, p.price_micros
FROM unnest(sqlc.arg(mints)::text[]) AS u (mint)
CROSS JOIN LATERAL (
  SELECT price_points.ts, price_points.price_micros
  FROM price_points
  WHERE price_points.mint = u.mint AND price_points.ts <= sqlc.arg(at)::timestamptz
  ORDER BY price_points.ts DESC
  LIMIT 3
) AS p;

-- name: FirstSamplesSince :many
SELECT u.mint::text AS mint, p.ts, p.price_micros
FROM unnest(sqlc.arg(mints)::text[]) AS u (mint)
CROSS JOIN LATERAL (
  (
    SELECT ts, price_micros
    FROM price_points
    WHERE mint = u.mint AND ts < sqlc.arg(since)::timestamptz
    ORDER BY ts DESC
    LIMIT 2
  )
  UNION ALL
  (
    SELECT ts, price_micros
    FROM price_points
    WHERE mint = u.mint
      AND ts >= sqlc.arg(since)::timestamptz
      AND ts <= sqlc.arg(until)::timestamptz
  )
) AS p
ORDER BY u.mint, p.ts ASC;

-- name: SparklineCloses :many
SELECT mint,
  date_bin(interval '30 minutes', ts, timestamptz '2000-01-01 00:00:00+00')::timestamptz AS bucket,
  (array_agg(price_micros ORDER BY ts DESC))[1]::bigint AS close_micros
FROM price_points
WHERE mint = ANY (sqlc.arg(mints)::text[])
  AND ts > sqlc.arg(since)::timestamptz
  AND ts <= sqlc.arg(until)::timestamptz
GROUP BY mint, bucket
ORDER BY mint, bucket;
