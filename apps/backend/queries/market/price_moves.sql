-- name: InsertAssetPriceMove :execrows
INSERT INTO asset_price_moves (asset_id, threshold_bps, trading_day, event_id, created_at)
VALUES (
  sqlc.arg(asset_id)::uuid,
  sqlc.arg(threshold_bps)::bigint,
  to_date(sqlc.arg(trading_day)::text, 'YYYY-MM-DD'),
  sqlc.arg(event_id)::uuid,
  sqlc.arg(created_at)::timestamptz
)
ON CONFLICT DO NOTHING;

-- name: DayPriceSamples :many
(
  SELECT p.ts, p.price_micros
  FROM price_points AS p
  JOIN assets AS a ON a.mint = p.mint
  WHERE a.id = sqlc.arg(asset_id)::uuid
    AND p.ts < sqlc.arg(day_start)::timestamptz
  ORDER BY p.ts DESC
  LIMIT sqlc.arg(lookback)::integer
)
UNION ALL
(
  SELECT p.ts, p.price_micros
  FROM price_points AS p
  JOIN assets AS a ON a.mint = p.mint
  WHERE a.id = sqlc.arg(asset_id)::uuid
    AND p.ts >= sqlc.arg(day_start)::timestamptz
    AND p.ts <= sqlc.arg(until)::timestamptz
)
ORDER BY ts ASC;
