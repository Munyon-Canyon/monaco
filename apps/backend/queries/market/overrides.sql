-- name: SetTradableOverride :one
UPDATE assets SET
  tradable_override = CASE sqlc.arg(override)::text WHEN 'auto' THEN NULL ELSE sqlc.arg(override)::text = 'on' END,
  updated_at = sqlc.arg(now)::timestamptz
WHERE symbol = sqlc.arg(symbol)::text
RETURNING id, symbol, mint, decimals, issuer, kind, display_name, logo_url, ui_multiplier_num, ui_multiplier_den, issuer_tradable, tradable_override, popular_rank, company_key, first_seen_at, updated_at, chain_checked_at, ui_multiplier_next_num, ui_multiplier_next_den, ui_multiplier_next_at;
