-- name: AssetByID :one
SELECT id, symbol, mint, decimals, issuer, kind, display_name, logo_url, ui_multiplier_num, ui_multiplier_den, issuer_tradable, tradable_override, popular_rank, company_key, first_seen_at, updated_at, chain_checked_at FROM assets WHERE id = $1;

-- name: AssetByMint :one
SELECT id, symbol, mint, decimals, issuer, kind, display_name, logo_url, ui_multiplier_num, ui_multiplier_den, issuer_tradable, tradable_override, popular_rank, company_key, first_seen_at, updated_at, chain_checked_at FROM assets WHERE mint = $1;

-- name: AssetBySymbol :one
SELECT id, symbol, mint, decimals, issuer, kind, display_name, logo_url, ui_multiplier_num, ui_multiplier_den, issuer_tradable, tradable_override, popular_rank, company_key, first_seen_at, updated_at, chain_checked_at FROM assets WHERE symbol = $1;

-- name: ListTradableAssets :many
SELECT id, symbol, mint, decimals, issuer, kind, display_name, logo_url, ui_multiplier_num, ui_multiplier_den, issuer_tradable, tradable_override, popular_rank, company_key, first_seen_at, updated_at, chain_checked_at FROM assets
WHERE chain_checked_at IS NOT NULL AND coalesce(tradable_override, issuer_tradable)
ORDER BY popular_rank NULLS LAST, symbol;

-- name: ListAssets :many
SELECT id, symbol, mint, decimals, issuer, kind, display_name, logo_url, ui_multiplier_num, ui_multiplier_den, issuer_tradable, tradable_override, popular_rank, company_key, first_seen_at, updated_at, chain_checked_at FROM assets ORDER BY symbol;

-- name: OtherListings :many
SELECT symbol::text AS symbol,
  display_name::text AS display_name,
  issuer::text AS issuer,
  kind::text AS kind,
  COALESCE(logo_url, '')::text AS logo_url,
  (chain_checked_at IS NOT NULL AND coalesce(tradable_override, issuer_tradable))::boolean AS tradable
FROM assets
WHERE company_key = sqlc.arg(company_key)::text
  AND symbol <> sqlc.arg(symbol)::text
ORDER BY symbol;
