-- name: HandleClaimFacts :one
SELECT
  u.handle,
  u.handle_changed_at,
  u.x_username,
  EXISTS (
    SELECT 1 FROM users o
    WHERE o.handle = sqlc.arg(handle)::text
      AND o.id <> u.id
  ) AS handle_taken,
  EXISTS (
    SELECT 1 FROM users o
    WHERE o.x_username IS NOT NULL
      AND lower(o.x_username) = sqlc.arg(handle)::text
      AND o.id <> u.id
  ) AS other_x_match
FROM users u
WHERE u.id = sqlc.arg(id) AND u.deleted_at IS NULL;
