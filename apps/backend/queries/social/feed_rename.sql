-- name: RenameFeedCabal :execrows
UPDATE feed_cabals SET name = sqlc.arg(name), updated_at = sqlc.arg(at) WHERE cabal_id = sqlc.arg(cabal_id);

-- name: StaleFeedByActor :many
SELECT id, kind, payload FROM feed_objects
WHERE actor_id = sqlc.arg(actor_id)::uuid AND payload->>'actor_name' <> sqlc.arg(name)::text
  AND id > sqlc.arg(after)::uuid
ORDER BY id
LIMIT sqlc.arg(row_limit)::int
FOR UPDATE;

-- name: StaleFeedByCabal :many
SELECT id, kind, payload FROM feed_objects
WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND cabal_name IS DISTINCT FROM sqlc.arg(name)::text
  AND id > sqlc.arg(after)::uuid
ORDER BY id
LIMIT sqlc.arg(row_limit)::int
FOR UPDATE;

-- name: RewriteFeedRows :exec
UPDATE feed_objects f
SET title = v.title, payload = v.payload, cabal_name = nullif(v.payload->>'cabal_name', ''),
  updated_at = sqlc.arg(at)
FROM (
  SELECT unnest(sqlc.arg(ids)::uuid[]) AS id, unnest(sqlc.arg(titles)::text[]) AS title,
    unnest(sqlc.arg(payloads)::jsonb[]) AS payload
) AS v
WHERE f.id = v.id;
