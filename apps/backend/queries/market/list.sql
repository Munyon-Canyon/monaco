-- name: ListAssetsBySymbol :many
SELECT * FROM assets
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
SELECT * FROM assets
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
