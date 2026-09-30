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
