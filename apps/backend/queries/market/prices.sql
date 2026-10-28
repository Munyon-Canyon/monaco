-- name: InsertPricePoints :many
INSERT INTO price_points (mint, ts, price_micros, source)
SELECT u.mint, sqlc.arg(ts)::timestamptz, u.price_micros, sqlc.arg(source)::text
FROM ROWS FROM (
  unnest(sqlc.arg(mints)::text[]), unnest(sqlc.arg(price_micros)::bigint[])
) AS u (mint, price_micros)
ON CONFLICT (mint, ts) DO NOTHING
RETURNING mint, ts, price_micros;

-- name: CountPricePoints :one
SELECT count(*)::bigint AS points FROM price_points;

-- name: RecentPriceSamples :many
SELECT a.id AS asset_id, p.ts, p.price_micros, p.source
FROM assets AS a
CROSS JOIN LATERAL (
  SELECT price_points.ts, price_points.price_micros, price_points.source
  FROM price_points
  WHERE price_points.mint = a.mint AND price_points.ts <= sqlc.arg(at)::timestamptz
  ORDER BY price_points.ts DESC
  LIMIT sqlc.arg(per_asset)::integer
) AS p
WHERE sqlc.arg(every_asset)::boolean OR a.id = ANY (sqlc.arg(ids)::uuid[])
ORDER BY a.id, p.ts DESC;

-- name: ThinPricePoints :many
WITH doomed AS (
  DELETE FROM price_points
  WHERE ctid IN (
    SELECT p.ctid
    FROM price_points AS p
    WHERE p.ts >= sqlc.arg(after)::timestamptz
      AND p.ts < sqlc.arg(older_than)::timestamptz
      AND p.ts < (SELECT max(latest.ts) FROM price_points AS latest WHERE latest.mint = p.mint)
      AND EXISTS (
        SELECT 1
        FROM price_points AS earlier
        WHERE earlier.mint = p.mint
          AND earlier.ts >= date_bin((sqlc.arg(bucket)::text)::interval, p.ts, TIMESTAMPTZ 'epoch')
          AND earlier.ts < p.ts
      )
    ORDER BY p.ts
    LIMIT sqlc.arg(batch_limit)::integer
  )
  RETURNING mint, ts, price_micros
),
rolled AS (
  INSERT INTO price_day_ranges (mint, day, high_micros, low_micros, source)
  SELECT d.mint, (d.ts AT TIME ZONE 'UTC')::date, max(d.price_micros), min(d.price_micros), 'samples'
  FROM doomed AS d
  LEFT JOIN LATERAL (
    SELECT prev.price_micros
    FROM price_points AS prev
    WHERE prev.mint = d.mint AND prev.ts < d.ts
    ORDER BY prev.ts DESC
    LIMIT 1
  ) AS earlier ON true
  WHERE d.price_micros > 0
    AND (earlier.price_micros IS NULL OR abs(d.price_micros - earlier.price_micros) <= earlier.price_micros / 5)
  GROUP BY d.mint, (d.ts AT TIME ZONE 'UTC')::date
  ON CONFLICT (mint, day) DO UPDATE SET
    high_micros = CASE
      WHEN price_day_ranges.source = 'history' THEN excluded.high_micros
      ELSE greatest(price_day_ranges.high_micros, excluded.high_micros)
    END,
    low_micros = CASE
      WHEN price_day_ranges.source = 'history' THEN excluded.low_micros
      ELSE least(price_day_ranges.low_micros, excluded.low_micros)
    END,
    source = 'samples'
)
SELECT ts FROM doomed;

-- name: ThinKeptPricePoints :many
WITH doomed AS (
  DELETE FROM price_points
  WHERE ctid IN (
    SELECT p.ctid
    FROM price_points AS p
    WHERE p.ts >= sqlc.arg(after)::timestamptz
      AND p.ts < sqlc.arg(older_than)::timestamptz
      AND (
        p.mint = ANY (sqlc.arg(keep)::text[])
        OR p.mint IN (SELECT a.mint FROM assets AS a WHERE a.popular_rank IS NOT NULL)
      )
      AND p.ts < (SELECT max(latest.ts) FROM price_points AS latest WHERE latest.mint = p.mint)
      AND EXISTS (
        SELECT 1
        FROM price_points AS earlier
        WHERE earlier.mint = p.mint
          AND earlier.ts >= date_bin((sqlc.arg(bucket)::text)::interval, p.ts, TIMESTAMPTZ 'epoch')
          AND earlier.ts < p.ts
      )
    ORDER BY p.ts
    LIMIT sqlc.arg(batch_limit)::integer
  )
  RETURNING mint, ts, price_micros
),
rolled AS (
  INSERT INTO price_day_ranges (mint, day, high_micros, low_micros, source)
  SELECT d.mint, (d.ts AT TIME ZONE 'UTC')::date, max(d.price_micros), min(d.price_micros), 'samples'
  FROM doomed AS d
  LEFT JOIN LATERAL (
    SELECT prev.price_micros
    FROM price_points AS prev
    WHERE prev.mint = d.mint AND prev.ts < d.ts
    ORDER BY prev.ts DESC
    LIMIT 1
  ) AS earlier ON true
  WHERE d.price_micros > 0
    AND (earlier.price_micros IS NULL OR abs(d.price_micros - earlier.price_micros) <= earlier.price_micros / 5)
  GROUP BY d.mint, (d.ts AT TIME ZONE 'UTC')::date
  ON CONFLICT (mint, day) DO UPDATE SET
    high_micros = CASE
      WHEN price_day_ranges.source = 'history' THEN excluded.high_micros
      ELSE greatest(price_day_ranges.high_micros, excluded.high_micros)
    END,
    low_micros = CASE
      WHEN price_day_ranges.source = 'history' THEN excluded.low_micros
      ELSE least(price_day_ranges.low_micros, excluded.low_micros)
    END,
    source = 'samples'
)
SELECT ts FROM doomed;

-- name: DeleteColdPricePoints :many
WITH doomed AS (
  DELETE FROM price_points
  WHERE ctid IN (
    SELECT p.ctid
    FROM price_points AS p
    WHERE p.ts < sqlc.arg(older_than)::timestamptz
      AND NOT (p.mint = ANY (sqlc.arg(keep)::text[]))
      AND p.mint NOT IN (SELECT a.mint FROM assets AS a WHERE a.popular_rank IS NOT NULL)
    LIMIT sqlc.arg(batch_limit)::integer
  )
  RETURNING mint, ts, price_micros
),
rolled AS (
  INSERT INTO price_day_ranges (mint, day, high_micros, low_micros, source)
  SELECT d.mint, (d.ts AT TIME ZONE 'UTC')::date, max(d.price_micros), min(d.price_micros), 'samples'
  FROM doomed AS d
  LEFT JOIN LATERAL (
    SELECT prev.price_micros
    FROM price_points AS prev
    WHERE prev.mint = d.mint AND prev.ts < d.ts
    ORDER BY prev.ts DESC
    LIMIT 1
  ) AS earlier ON true
  WHERE d.price_micros > 0
    AND (earlier.price_micros IS NULL OR abs(d.price_micros - earlier.price_micros) <= earlier.price_micros / 5)
  GROUP BY d.mint, (d.ts AT TIME ZONE 'UTC')::date
  ON CONFLICT (mint, day) DO UPDATE SET
    high_micros = CASE
      WHEN price_day_ranges.source = 'history' THEN excluded.high_micros
      ELSE greatest(price_day_ranges.high_micros, excluded.high_micros)
    END,
    low_micros = CASE
      WHEN price_day_ranges.source = 'history' THEN excluded.low_micros
      ELSE least(price_day_ranges.low_micros, excluded.low_micros)
    END,
    source = 'samples'
)
SELECT ts FROM doomed;

-- name: InsertReconciledPricePoints :execrows
INSERT INTO price_points (mint, ts, price_micros, source)
SELECT sqlc.arg(mint)::text, u.ts, u.price_micros, sqlc.arg(source)::text
FROM ROWS FROM (
  unnest(sqlc.arg(timestamps)::timestamptz[]), unnest(sqlc.arg(price_micros)::bigint[])
) AS u (ts, price_micros)
ON CONFLICT (mint, ts) DO NOTHING;

-- name: MarkAssetsQuoted :execrows
UPDATE assets SET last_quoted_at = sqlc.arg(at)::timestamptz
WHERE mint = ANY (sqlc.arg(mints)::text[])
  AND (last_quoted_at IS NULL OR last_quoted_at < sqlc.arg(at)::timestamptz);

-- name: UpsertHistoryDayRanges :exec
INSERT INTO price_day_ranges (mint, day, high_micros, low_micros, source)
SELECT sqlc.arg(mint)::text, (u.day AT TIME ZONE 'UTC')::date, u.high_micros, u.low_micros, 'history'
FROM ROWS FROM (
  unnest(sqlc.arg(days)::timestamptz[]), unnest(sqlc.arg(high_micros)::bigint[]), unnest(sqlc.arg(low_micros)::bigint[])
) AS u (day, high_micros, low_micros)
ON CONFLICT (mint, day) DO UPDATE SET
  high_micros = excluded.high_micros,
  low_micros = excluded.low_micros
WHERE price_day_ranges.source = 'history';
