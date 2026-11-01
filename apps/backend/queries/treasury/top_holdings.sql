-- name: TopHoldings :many
WITH priced AS (
  SELECT p.cabal_id, p.asset, p.cost_basis_micros,
    CASE
      WHEN a.id IS NOT NULL THEN
        p.units * px.price_micros * a.ui_multiplier_num / (power(10::numeric, a.decimals) * a.ui_multiplier_den)
      WHEN l.id IS NOT NULL THEN
        p.units * l.shares_per_token * px.price_micros / power(10::numeric, l.decimals)
    END AS value_micros
  FROM cabal_positions AS p
  LEFT JOIN assets AS a ON a.mint = p.asset
  LEFT JOIN asset_listings AS l ON l.id = p.listing_id AND a.id IS NULL
  LEFT JOIN LATERAL (
    SELECT price_points.price_micros
    FROM price_points
    WHERE price_points.mint = p.asset AND price_points.ts <= now() AND price_points.ts > now() - interval '5 minutes'
    ORDER BY price_points.ts DESC
    LIMIT 1
  ) AS px ON true
  WHERE p.cabal_id = ANY(@cabal_ids::uuid[]) AND p.units > 0
)
SELECT DISTINCT ON (cabal_id) cabal_id, asset
FROM priced
ORDER BY cabal_id, coalesce(value_micros, cost_basis_micros) DESC, asset;
