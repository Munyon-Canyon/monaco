-- name: InsertBroadcast :one
INSERT INTO notification_broadcasts (id, kind, source_event_id, recipient_count, created_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (source_event_id) DO NOTHING
RETURNING id;

-- name: InsertNotification :one
INSERT INTO notifications (
  id, broadcast_id, user_id, kind, source_event_id, title, body, data, collapse_id, state, created_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (source_event_id, user_id, kind) DO NOTHING
RETURNING id;

-- name: UndeliveredForEvent :many
SELECT id, broadcast_id, user_id, kind, title, body, data, collapse_id, created_at FROM notifications
WHERE source_event_id = $1 AND state = 'pending'
ORDER BY created_at, id;

-- name: MarkDelivered :execrows
UPDATE notifications SET state = 'delivered', delivered_at = sqlc.arg(delivered_at)::timestamptz
WHERE id = sqlc.arg(id) AND state = 'pending';

-- name: MarkNoDevice :execrows
UPDATE notifications SET state = 'no_device'
WHERE id = sqlc.arg(id) AND state = 'pending';

-- name: CountKindSince :one
SELECT count(*) FROM notifications
WHERE user_id = $1 AND kind = $2 AND created_at >= $3;
