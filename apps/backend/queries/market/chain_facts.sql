-- name: AssetsDueForChainCheck :many
SELECT id, symbol, mint, decimals, issuer, kind, display_name, logo_url, ui_multiplier_num, ui_multiplier_den, issuer_tradable, tradable_override, popular_rank, company_key, first_seen_at, updated_at, chain_checked_at, ui_multiplier_next_num, ui_multiplier_next_den, ui_multiplier_next_at FROM assets
WHERE chain_checked_at IS NULL OR chain_checked_at < sqlc.arg(stale_before)::timestamptz
ORDER BY chain_checked_at NULLS FIRST, coalesce(tradable_override, issuer_tradable) DESC, popular_rank NULLS LAST, symbol
LIMIT sqlc.arg(max_assets);

-- name: StoreChainFacts :many
UPDATE assets SET
  decimals = f.chain_decimals,
  ui_multiplier_num = f.multiplier_num,
  ui_multiplier_den = f.multiplier_den,
  ui_multiplier_next_num = CASE WHEN f.next_num = 0 THEN NULL ELSE f.next_num END,
  ui_multiplier_next_den = CASE WHEN f.next_num = 0 THEN NULL ELSE f.next_den END,
  ui_multiplier_next_at = CASE WHEN f.next_num = 0 THEN NULL ELSE f.next_at END,
  chain_checked_at = sqlc.arg(now)::timestamptz,
  updated_at = sqlc.arg(now)::timestamptz
FROM ROWS FROM (
  unnest(sqlc.arg(mints)::text[]), unnest(sqlc.arg(issuer_decimals)::smallint[]),
  unnest(sqlc.arg(chain_decimals)::smallint[]), unnest(sqlc.arg(multiplier_nums)::bigint[]),
  unnest(sqlc.arg(multiplier_dens)::bigint[]), unnest(sqlc.arg(next_nums)::bigint[]),
  unnest(sqlc.arg(next_dens)::bigint[]), unnest(sqlc.arg(next_ats)::timestamptz[]),
  unnest(sqlc.arg(rechecks)::bool[]), unnest(sqlc.arg(checked_ats)::timestamptz[])
) AS f (
  mint, issuer_decimals, chain_decimals, multiplier_num, multiplier_den, next_num, next_den, next_at, recheck, checked_at
)
WHERE assets.mint = f.mint AND assets.decimals = f.issuer_decimals
  AND CASE WHEN f.recheck THEN assets.chain_checked_at = f.checked_at ELSE assets.chain_checked_at IS NULL END
RETURNING assets.mint;
