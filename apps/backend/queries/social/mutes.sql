-- name: FeedMuteLabel :one
SELECT COALESCE(CASE sqlc.arg(target_type)::text
  WHEN 'cabal' THEN f.cabal_name
  WHEN 'asset' THEN f.symbol
  WHEN 'user' THEN f.payload->>'actor_name'
  WHEN 'item' THEN f.title
END, '')::text AS label
FROM feed_objects f
WHERE (sqlc.arg(target_type)::text = 'cabal' AND f.cabal_id = sqlc.arg(target_id)::uuid)
   OR (sqlc.arg(target_type)::text = 'asset' AND f.asset_id = sqlc.arg(target_id)::uuid)
   OR (sqlc.arg(target_type)::text = 'user' AND f.actor_id = sqlc.arg(target_id)::uuid)
   OR (sqlc.arg(target_type)::text = 'item' AND f.id = sqlc.arg(target_id)::uuid)
ORDER BY f.created_at DESC, f.id DESC
LIMIT 1;

-- name: InsertFeedMute :exec
INSERT INTO feed_mutes (user_id, target_type, target_id, label, created_at)
VALUES (sqlc.arg(user_id), sqlc.arg(target_type), sqlc.arg(target_id), sqlc.arg(label)::text, sqlc.arg(created_at))
ON CONFLICT DO NOTHING;

-- name: DeleteFeedMute :exec
DELETE FROM feed_mutes
WHERE user_id = sqlc.arg(user_id) AND target_type = sqlc.arg(target_type) AND target_id = sqlc.arg(target_id);

-- name: ListFeedMutes :many
SELECT target_type, target_id, label, created_at
FROM feed_mutes
WHERE user_id = sqlc.arg(user_id)
ORDER BY created_at DESC, target_type, target_id;
