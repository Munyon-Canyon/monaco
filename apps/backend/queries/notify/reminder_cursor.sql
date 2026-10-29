-- name: GetReminderCursor :one
SELECT due_at, proposal_id FROM notify_reminder_cursor WHERE name = @name;

-- name: AdvanceReminderCursor :exec
INSERT INTO notify_reminder_cursor (name, due_at, proposal_id)
VALUES (@name, @due_at, @proposal_id)
ON CONFLICT (name) DO UPDATE SET due_at = EXCLUDED.due_at, proposal_id = EXCLUDED.proposal_id
WHERE (notify_reminder_cursor.due_at, notify_reminder_cursor.proposal_id) < (EXCLUDED.due_at, EXCLUDED.proposal_id);

-- name: UsersWithNotification :many
SELECT user_id FROM notifications WHERE source_event_id = @source_event_id AND kind = @kind;
