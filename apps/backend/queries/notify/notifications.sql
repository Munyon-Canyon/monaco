-- name: InsertBroadcast :one
INSERT INTO notification_broadcasts (id, kind, source_event_id, recipient_count, created_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (source_event_id) DO NOTHING
RETURNING id;

-- name: InsertNotification :one
INSERT INTO notifications (
  id, broadcast_id, user_id, kind, source_event_id, title, body, data, collapse_id, state, created_at
)
VALUES (
  sqlc.arg(id), NULLIF(sqlc.arg(broadcast_id)::uuid, '00000000-0000-0000-0000-000000000000'), sqlc.arg(user_id),
  sqlc.arg(kind), sqlc.arg(source_event_id), sqlc.arg(title), sqlc.arg(body), sqlc.arg(data), sqlc.arg(collapse_id),
  sqlc.arg(state), sqlc.arg(created_at)
)
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

-- name: BatchedFollowCounts :many
SELECT user_id, count(*) AS followers FROM notifications
WHERE kind = 'new_follower' AND state = 'batched'
  AND created_at >= sqlc.arg(since)::timestamptz AND created_at < sqlc.arg(until)::timestamptz
GROUP BY user_id
ORDER BY user_id;

-- name: EventActor :one
SELECT actor_type, actor_id FROM events WHERE id = $1;
