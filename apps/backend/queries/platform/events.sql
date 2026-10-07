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

-- name: DeliveryExists :one
SELECT EXISTS(SELECT 1 FROM event_deliveries WHERE handler = $1 AND event_id = $2);

-- name: DeleteDeliveriesBefore :execrows
DELETE FROM event_deliveries
WHERE (handler, event_id) IN (
  SELECT handler, event_id FROM event_deliveries WHERE handled_at < @before::timestamptz LIMIT @batch
);

-- name: Backlog :one
SELECT count(*)::bigint AS unpublished, coalesce(min(created_at), @now::timestamptz)::timestamptz AS oldest_created_at
FROM events
WHERE published_at IS NULL;

-- name: EventsByAggregate :many
SELECT id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id, created_at, published_at
FROM events
WHERE aggregate_type = @aggregate_type AND aggregate_id = @aggregate_id
  AND (cardinality(@types::text[]) = 0 OR type = ANY(@types::text[]))
ORDER BY id
LIMIT @row_limit::bigint;

-- name: ListStaleUnpublished :many
SELECT id, aggregate_type, aggregate_id, type, created_at
FROM events
WHERE published_at IS NULL AND created_at < @cutoff::timestamptz
ORDER BY id
LIMIT @row_limit::bigint;

-- name: CountStaleUnpublished :one
SELECT count(*)::bigint
FROM events
WHERE published_at IS NULL AND created_at < @cutoff::timestamptz;
