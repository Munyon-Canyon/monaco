-- name: CatchUpRows :many
SELECT id, kind, item_key, data, created_at FROM notifications
WHERE user_id = @user_id AND kind = ANY(@kinds::text[]) AND item_key IS NOT NULL
  AND created_at >= @since::timestamptz
ORDER BY created_at DESC, id DESC
LIMIT 200;

-- name: CatchUpState :many
SELECT item_key, read_at, cleared_at, cleared_reason, kept_at FROM catch_up_state
WHERE user_id = @user_id AND item_key = ANY(@keys::text[]);

-- name: ClearedSince :many
SELECT item_key, cleared_at, cleared_reason, snapshot FROM catch_up_state
WHERE user_id = @user_id AND cleared_at >= @since::timestamptz
ORDER BY cleared_at DESC, item_key;
