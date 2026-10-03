-- name: ListAssetsBySymbol :many
SELECT id, symbol, mint, decimals, issuer, kind, display_name, logo_url, ui_multiplier_num, ui_multiplier_den, issuer_tradable, tradable_override, popular_rank, company_key, first_seen_at, updated_at, chain_checked_at FROM assets
WHERE chain_checked_at IS NOT NULL
  AND coalesce(tradable_override, issuer_tradable)
  AND (sqlc.arg(kind)::text = '' OR kind = sqlc.arg(kind)::text)
  AND (
    sqlc.arg(q)::text = ''
    OR symbol ILIKE sqlc.arg(prefix)::text ESCAPE '!'
    OR display_name ILIKE sqlc.arg(contains)::text ESCAPE '!'
  )
  AND (
    NOT sqlc.arg(has_cursor)::boolean
    OR (symbol, id) > (sqlc.arg(cursor_symbol)::text, sqlc.arg(cursor_id)::uuid)
  )
ORDER BY symbol, id
LIMIT sqlc.arg(row_limit)::integer;

-- name: ListAssetsByRank :many
SELECT id, symbol, mint, decimals, issuer, kind, display_name, logo_url, ui_multiplier_num, ui_multiplier_den, issuer_tradable, tradable_override, popular_rank, company_key, first_seen_at, updated_at, chain_checked_at FROM assets
WHERE chain_checked_at IS NOT NULL
  AND coalesce(tradable_override, issuer_tradable)
  AND popular_rank IS NOT NULL
  AND (
    sqlc.arg(q)::text = ''
    OR symbol ILIKE sqlc.arg(prefix)::text ESCAPE '!'
    OR display_name ILIKE sqlc.arg(contains)::text ESCAPE '!'
  )
  AND (
    NOT sqlc.arg(has_cursor)::boolean
    OR (popular_rank, id) > (sqlc.arg(cursor_rank)::smallint, sqlc.arg(cursor_id)::uuid)
  )
ORDER BY popular_rank, id
LIMIT sqlc.arg(row_limit)::integer;
