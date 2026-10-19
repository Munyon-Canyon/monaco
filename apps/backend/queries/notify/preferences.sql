-- name: GetPreferences :many
SELECT user_id, category, enabled FROM notification_preferences
WHERE user_id = ANY(sqlc.arg(user_ids)::uuid[]);

-- name: UpsertPreference :exec
INSERT INTO notification_preferences (user_id, category, enabled, updated_at)
VALUES (sqlc.arg(user_id), sqlc.arg(category), sqlc.arg(enabled), sqlc.arg(updated_at))
ON CONFLICT (user_id, category) DO UPDATE
SET enabled = excluded.enabled, updated_at = excluded.updated_at;
