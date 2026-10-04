-- name: SearchUsers :many
SELECT id, handle, display_name, photo_url,
  handle LIKE sqlc.arg(prefix)::text ESCAPE '!' AS handle_match
FROM users
WHERE account_status NOT IN ('banned', 'deleted')
  AND deleted_at IS NULL
  AND handle IS NOT NULL
  AND id <> sqlc.arg(caller)::uuid
  AND (
    handle LIKE sqlc.arg(prefix)::text ESCAPE '!'
    OR lower(display_name) LIKE sqlc.arg(contains)::text ESCAPE '!'
  )
ORDER BY handle_match DESC, handle
LIMIT 20;
