-- name: SetTradableOverride :one
UPDATE assets SET
  tradable_override = CASE sqlc.arg(override)::text WHEN 'auto' THEN NULL ELSE sqlc.arg(override)::text = 'on' END,
  updated_at = sqlc.arg(now)::timestamptz
WHERE symbol = sqlc.arg(symbol)::text
RETURNING *;
