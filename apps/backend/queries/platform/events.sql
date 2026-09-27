-- name: AppendEvent :exec
INSERT INTO events (id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id, trace_parent, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ListUnpublished :many
SELECT id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id, trace_parent, created_at
FROM events
WHERE published_at IS NULL
ORDER BY id
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkPublished :execrows
UPDATE events SET published_at = @published_at::timestamptz WHERE id = ANY(@ids::uuid[]) AND published_at IS NULL;

-- name: InsertDelivery :execrows
INSERT INTO event_deliveries (handler, event_id, code, handled_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING;

-- name: DeleteDeliveriesBefore :execrows
DELETE FROM event_deliveries WHERE handled_at < $1;
