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

-- name: UpsertRead :exec
INSERT INTO catch_up_state (user_id, item_key, read_at, updated_at)
VALUES (@user_id, @item_key, @at::timestamptz, @at::timestamptz)
ON CONFLICT (user_id, item_key) DO UPDATE
SET read_at = EXCLUDED.read_at, updated_at = EXCLUDED.updated_at
WHERE catch_up_state.updated_at <= EXCLUDED.updated_at;

-- name: UpsertClear :exec
INSERT INTO catch_up_state (user_id, item_key, cleared_at, cleared_reason, snapshot, updated_at)
VALUES (@user_id, @item_key, @at::timestamptz, @reason::text, @snapshot::jsonb, @at::timestamptz)
ON CONFLICT (user_id, item_key) DO UPDATE
SET cleared_at = EXCLUDED.cleared_at, cleared_reason = EXCLUDED.cleared_reason, snapshot = EXCLUDED.snapshot,
    updated_at = EXCLUDED.updated_at
WHERE catch_up_state.updated_at <= EXCLUDED.updated_at;

-- name: UpsertKeep :exec
INSERT INTO catch_up_state (user_id, item_key, read_at, kept_at, updated_at)
VALUES (@user_id, @item_key, @at::timestamptz, @at::timestamptz, @at::timestamptz)
ON CONFLICT (user_id, item_key) DO UPDATE
SET read_at = EXCLUDED.read_at, kept_at = EXCLUDED.kept_at, updated_at = EXCLUDED.updated_at
WHERE catch_up_state.updated_at <= EXCLUDED.updated_at;

-- name: ReopenVoted :exec
INSERT INTO catch_up_state (user_id, item_key, updated_at)
VALUES (@user_id, @item_key, @at::timestamptz)
ON CONFLICT (user_id, item_key) DO UPDATE
SET cleared_at = CASE WHEN catch_up_state.cleared_reason = 'voted' THEN NULL ELSE catch_up_state.cleared_at END,
    snapshot = CASE WHEN catch_up_state.cleared_reason = 'voted' THEN NULL ELSE catch_up_state.snapshot END,
    cleared_reason = CASE WHEN catch_up_state.cleared_reason = 'voted' THEN NULL ELSE catch_up_state.cleared_reason END,
    updated_at = EXCLUDED.updated_at
WHERE catch_up_state.updated_at <= EXCLUDED.updated_at;

-- name: ThreadCommentKeys :many
SELECT DISTINCT item_key::text AS item_key FROM notifications
WHERE user_id = @user_id AND item_key LIKE 'comment:%' AND data ->> 'feed_item_id' = @feed_item_id::text
  AND created_at >= @since::timestamptz
ORDER BY 1;
