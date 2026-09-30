-- name: GetEventOrigin :one
SELECT actor_type, actor_id, created_at
FROM events
WHERE id = $1;
