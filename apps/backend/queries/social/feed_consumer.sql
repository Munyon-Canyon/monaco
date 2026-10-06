-- name: UpsertFeedCabal :exec
INSERT INTO feed_cabals (cabal_id, name, updated_at)
VALUES (sqlc.arg(cabal_id), sqlc.arg(name), sqlc.arg(at))
ON CONFLICT (cabal_id) DO UPDATE SET name = excluded.name, updated_at = excluded.updated_at;

-- name: UpsertFeedMembership :execrows
INSERT INTO feed_memberships (cabal_id, user_id, joined_at, event_id, active)
VALUES (sqlc.arg(cabal_id), sqlc.arg(user_id), sqlc.arg(joined_at), sqlc.arg(event_id), true)
ON CONFLICT (cabal_id, user_id) DO UPDATE SET
  joined_at = excluded.joined_at,
  event_id = excluded.event_id,
  active = true
WHERE feed_memberships.event_id < excluded.event_id;

-- name: DeleteFeedMembership :execrows
INSERT INTO feed_memberships (cabal_id, user_id, joined_at, event_id, active)
VALUES (sqlc.arg(cabal_id), sqlc.arg(user_id), sqlc.arg(at), sqlc.arg(event_id), false)
ON CONFLICT (cabal_id, user_id) DO UPDATE SET
  event_id = excluded.event_id,
  active = false
WHERE feed_memberships.event_id < excluded.event_id;

-- name: FeedCabalExists :one
SELECT EXISTS(SELECT 1 FROM feed_cabals WHERE cabal_id = sqlc.arg(cabal_id));

-- name: FeedCabalName :one
SELECT name FROM feed_cabals WHERE cabal_id = sqlc.arg(cabal_id);

-- name: InsertFeedConsumerItem :exec
INSERT INTO feed_objects (
  id, kind, ref_type, ref_id, cabal_id, cabal_name, actor_id, asset_id, symbol, title, payload, status,
  created_at, updated_at
)
VALUES (
  sqlc.arg(id), sqlc.arg(kind), sqlc.arg(ref_type), sqlc.arg(ref_id), sqlc.arg(cabal_id), sqlc.arg(cabal_name),
  sqlc.arg(actor_id), sqlc.narg(asset_id), sqlc.narg(symbol), sqlc.arg(title), sqlc.arg(payload),
  sqlc.narg(status), sqlc.arg(at), sqlc.arg(at)
)
ON CONFLICT (ref_type, ref_id, kind) DO NOTHING;

-- name: AdvanceFeedProposal :one
WITH moved AS (
  UPDATE feed_objects
  SET status = sqlc.arg(to_status)::text, payload = payload || sqlc.arg(patch)::jsonb, updated_at = sqlc.arg(at)
  WHERE ref_type = 'proposals' AND ref_id = sqlc.arg(proposal_id)::uuid AND kind = 'proposal'
    AND status = ANY(sqlc.arg(from_statuses)::text[])
  RETURNING 1
)
SELECT
  (SELECT count(*) FROM moved)::int AS moved,
  EXISTS(
    SELECT 1 FROM feed_objects WHERE ref_type = 'proposals' AND ref_id = sqlc.arg(proposal_id)::uuid AND kind = 'proposal'
  ) AS found;
