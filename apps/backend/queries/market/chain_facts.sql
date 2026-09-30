-- name: UncheckedAssets :many
SELECT * FROM assets
WHERE chain_checked_at IS NULL
ORDER BY coalesce(tradable_override, issuer_tradable) DESC, popular_rank NULLS LAST, symbol
LIMIT sqlc.arg(max_assets);

-- name: StoreChainFacts :many
UPDATE assets SET
  decimals = f.chain_decimals,
  ui_multiplier_num = f.multiplier_num,
  ui_multiplier_den = f.multiplier_den,
  chain_checked_at = sqlc.arg(now)::timestamptz,
  updated_at = sqlc.arg(now)::timestamptz
FROM ROWS FROM (
  unnest(sqlc.arg(mints)::text[]), unnest(sqlc.arg(issuer_decimals)::smallint[]),
  unnest(sqlc.arg(chain_decimals)::smallint[]), unnest(sqlc.arg(multiplier_nums)::bigint[]),
  unnest(sqlc.arg(multiplier_dens)::bigint[])
) AS f (mint, issuer_decimals, chain_decimals, multiplier_num, multiplier_den)
WHERE assets.mint = f.mint AND assets.chain_checked_at IS NULL AND assets.decimals = f.issuer_decimals
RETURNING assets.mint;
