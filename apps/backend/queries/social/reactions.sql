-- name: AddReaction :one
INSERT INTO feed_reactions (feed_object_id, user_id, emoji, created_at)
VALUES (sqlc.arg(feed_object_id), sqlc.arg(user_id), sqlc.arg(emoji), sqlc.arg(created_at))
ON CONFLICT (feed_object_id, user_id, emoji) DO NOTHING
RETURNING created_at;

-- name: RemoveReaction :execrows
DELETE FROM feed_reactions
WHERE feed_object_id = sqlc.arg(feed_object_id) AND user_id = sqlc.arg(user_id) AND emoji = sqlc.arg(emoji);

-- name: CountUserReactionsOnItem :one
SELECT count(*)::int FROM feed_reactions
WHERE feed_object_id = sqlc.arg(feed_object_id) AND user_id = sqlc.arg(user_id);

-- name: ReactionSummaries :many
WITH counted AS (
  SELECT r.feed_object_id, r.emoji, count(*)::int AS count, bool_or(r.user_id = sqlc.arg(viewer)::uuid) AS mine,
    min(r.created_at) AS first_at
  FROM feed_reactions r
  WHERE r.feed_object_id = ANY(sqlc.arg(feed_object_ids)::uuid[])
  GROUP BY r.feed_object_id, r.emoji
), ranked AS (
  SELECT c.feed_object_id, c.emoji, c.count, c.mine,
    row_number() OVER (PARTITION BY c.feed_object_id ORDER BY c.count DESC, c.first_at, c.emoji) AS rank,
    sum(c.count) OVER (PARTITION BY c.feed_object_id)::int AS total
  FROM counted c
)
SELECT ranked.feed_object_id, ranked.emoji, ranked.count, ranked.mine::bool AS mine, ranked.total
FROM ranked
WHERE ranked.rank <= 20
ORDER BY ranked.feed_object_id, ranked.rank;
