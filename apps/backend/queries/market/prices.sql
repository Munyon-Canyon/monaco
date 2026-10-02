-- name: InsertPricePoints :execrows
INSERT INTO price_points (mint, ts, price_micros, source)
SELECT u.mint, sqlc.arg(ts)::timestamptz, u.price_micros, sqlc.arg(source)::text
FROM ROWS FROM (
  unnest(sqlc.arg(mints)::text[]), unnest(sqlc.arg(price_micros)::bigint[])
) AS u (mint, price_micros)
ON CONFLICT (mint, ts) DO NOTHING;

-- name: RecentPriceSamples :many
SELECT a.id AS asset_id, p.ts, p.price_micros
FROM assets AS a
CROSS JOIN LATERAL (
  SELECT price_points.ts, price_points.price_micros
  FROM price_points
  WHERE price_points.mint = a.mint AND price_points.ts <= sqlc.arg(at)::timestamptz
  ORDER BY price_points.ts DESC
  LIMIT sqlc.arg(per_asset)::integer
) AS p
WHERE sqlc.arg(every_asset)::boolean OR a.id = ANY (sqlc.arg(ids)::uuid[])
ORDER BY a.id, p.ts DESC;

-- name: ThinPricePoints :many
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
RETURNING ts;
