-- name: ListingPriceSamples :many
SELECT l.id, l.asset_id, l.issuer, l.symbol, l.network, l.token_address, l.decimals,
  l.shares_per_token::text AS shares_per_token, l.venue, (l.network || ':' || l.token_address)::text AS key,
  a.mint AS asset_mint, a.symbol AS asset_symbol, a.display_name AS asset_display_name, a.kind AS asset_kind,
  (p.ts IS NOT NULL)::bool AS sampled, coalesce(p.ts, sqlc.arg(at)::timestamptz)::timestamptz AS ts,
  coalesce(p.price_micros, 0)::bigint AS price_micros
FROM asset_listings AS l
LEFT JOIN assets AS a ON a.id = l.asset_id
LEFT JOIN LATERAL (
  SELECT price_points.ts, price_points.price_micros
  FROM price_points
  WHERE price_points.mint = l.network || ':' || l.token_address AND price_points.ts <= sqlc.arg(at)::timestamptz
  ORDER BY price_points.ts DESC
  LIMIT sqlc.arg(per_key)::integer
) AS p ON true
WHERE (l.network || ':' || l.token_address) = ANY (sqlc.arg(keys)::text[]) AND l.decimals IS NOT NULL
ORDER BY key, p.ts DESC;
