-- name: UpsertFeedItem :one
INSERT INTO feed_objects (
  id, kind, ref_type, ref_id, cabal_id, cabal_name, actor_id, asset_id, symbol, title, body, payload, status,
  created_at, updated_at
)
VALUES (
  sqlc.arg(id), sqlc.arg(kind), sqlc.arg(ref_type), sqlc.arg(ref_id), sqlc.narg(cabal_id), sqlc.narg(cabal_name),
  sqlc.narg(actor_id), sqlc.narg(asset_id), sqlc.narg(symbol), sqlc.arg(title), sqlc.narg(body), sqlc.arg(payload),
  sqlc.narg(status), sqlc.arg(at), sqlc.arg(at)
)
ON CONFLICT (ref_type, ref_id, kind) DO UPDATE SET
  cabal_id = excluded.cabal_id, cabal_name = excluded.cabal_name, actor_id = excluded.actor_id,
  asset_id = excluded.asset_id, symbol = excluded.symbol, title = excluded.title, body = excluded.body,
  payload = excluded.payload, updated_at = excluded.updated_at
RETURNING id;

-- name: UpdateFeedStatus :execrows
UPDATE feed_objects
SET status = sqlc.arg(to_status)::text, title = sqlc.arg(title), payload = sqlc.arg(payload),
  updated_at = sqlc.arg(at)
WHERE ref_type = sqlc.arg(ref_type) AND ref_id = sqlc.arg(ref_id) AND kind = sqlc.arg(kind)
  AND status = sqlc.arg(from_status)::text;

-- name: ListFeed :many
SELECT f.id, f.kind, f.ref_type, f.ref_id, f.cabal_id, f.actor_id, f.symbol, f.title, f.body, f.payload, f.status,
  f.comment_count, f.created_at, f.updated_at
FROM feed_objects f
WHERE (cardinality(sqlc.arg(kinds)::text[]) = 0 OR f.kind = ANY(sqlc.arg(kinds)::text[]))
  AND (sqlc.arg(cabal_id)::uuid = '00000000-0000-0000-0000-000000000000' OR f.cabal_id = sqlc.arg(cabal_id)::uuid)
  AND (sqlc.arg(symbol)::text = '' OR lower(f.symbol) = lower(sqlc.arg(symbol)::text))
  AND (
    sqlc.arg(q)::text = ''
    OR f.search @@ websearch_to_tsquery('english', sqlc.arg(q)::text)
    OR f.search @@ websearch_to_tsquery('simple', sqlc.arg(q)::text)
  )
  AND (
    NOT sqlc.arg(following)::bool
    OR f.actor_id IN (
      SELECT followee_id FROM follows WHERE follower_id = sqlc.arg(viewer)::uuid AND deleted_at IS NULL
    )
  )
  AND (
    NOT sqlc.arg(has_cursor)::bool
    OR (f.created_at, f.id) < (sqlc.arg(after_at)::timestamptz, sqlc.arg(after_id)::uuid)
  )
ORDER BY f.created_at DESC, f.id DESC
LIMIT sqlc.arg(row_limit)::int;
