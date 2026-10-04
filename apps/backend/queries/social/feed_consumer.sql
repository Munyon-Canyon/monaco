-- name: UpsertFeedCabal :exec
INSERT INTO feed_cabals (cabal_id, name, updated_at)
VALUES (sqlc.arg(cabal_id), sqlc.arg(name), sqlc.arg(at))
ON CONFLICT (cabal_id) DO UPDATE SET name = excluded.name, updated_at = excluded.updated_at;

-- name: UpsertFeedMembership :exec
INSERT INTO feed_memberships (cabal_id, user_id, joined_at)
VALUES (sqlc.arg(cabal_id), sqlc.arg(user_id), sqlc.arg(joined_at))
ON CONFLICT (cabal_id, user_id) DO UPDATE SET joined_at = excluded.joined_at;

-- name: DeleteFeedMembership :exec
DELETE FROM feed_memberships
WHERE cabal_id = sqlc.arg(cabal_id) AND user_id = sqlc.arg(user_id);

-- name: InsertFeedConsumerItem :exec
INSERT INTO feed_objects (
  id, kind, ref_type, ref_id, cabal_id, cabal_name, actor_id, title, payload, created_at, updated_at
)
VALUES (
  sqlc.arg(id), sqlc.arg(kind), sqlc.arg(ref_type), sqlc.arg(ref_id), sqlc.arg(cabal_id), sqlc.arg(cabal_name),
  sqlc.arg(actor_id), sqlc.arg(title), sqlc.arg(payload), sqlc.arg(at), sqlc.arg(at)
)
ON CONFLICT (ref_type, ref_id, kind) DO NOTHING;
