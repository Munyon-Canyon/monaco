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

-- name: AddCommentReaction :one
INSERT INTO comment_reactions (comment_id, feed_object_id, user_id, emoji, created_at)
VALUES (sqlc.arg(comment_id), sqlc.arg(feed_object_id), sqlc.arg(user_id), sqlc.arg(emoji), sqlc.arg(created_at))
ON CONFLICT (comment_id, user_id, emoji) DO NOTHING
RETURNING created_at;

-- name: RemoveCommentReaction :execrows
DELETE FROM comment_reactions
WHERE comment_id = sqlc.arg(comment_id) AND user_id = sqlc.arg(user_id) AND emoji = sqlc.arg(emoji);

-- name: CountUserCommentReactions :one
SELECT count(*)::int FROM comment_reactions
WHERE comment_id = sqlc.arg(comment_id) AND user_id = sqlc.arg(user_id);

-- name: DeleteCommentReactions :execrows
DELETE FROM comment_reactions WHERE comment_id = sqlc.arg(comment_id);

-- name: CommentReactionSummaries :many
WITH counted AS (
  SELECT r.comment_id, r.emoji, count(*)::int AS count, bool_or(r.user_id = sqlc.arg(viewer)::uuid) AS mine,
    min(r.created_at) AS first_at
  FROM comment_reactions r
  WHERE r.comment_id = ANY(sqlc.arg(comment_ids)::uuid[])
  GROUP BY r.comment_id, r.emoji
), ranked AS (
  SELECT c.comment_id, c.emoji, c.count, c.mine,
    row_number() OVER (PARTITION BY c.comment_id ORDER BY c.count DESC, c.first_at, c.emoji) AS rank,
    sum(c.count) OVER (PARTITION BY c.comment_id)::int AS total
  FROM counted c
)
SELECT ranked.comment_id, ranked.emoji, ranked.count, ranked.mine::bool AS mine, ranked.total
FROM ranked
WHERE ranked.rank <= 20
ORDER BY ranked.comment_id, ranked.rank;

-- name: TopReplyBetween :one
SELECT c.id AS comment_id, o.ref_id AS proposal_id, c.author_id, c.body, count(*)::int AS reaction_count
FROM comment_reactions r
JOIN feed_comments c ON c.id = r.comment_id AND c.deleted_at IS NULL
JOIN feed_objects o ON o.id = c.feed_object_id AND o.ref_type = 'proposals' AND o.kind = 'proposal'
WHERE o.ref_id = ANY(sqlc.arg(proposal_ids)::uuid[])
  AND r.created_at >= sqlc.arg(window_start)::timestamptz AND r.created_at < sqlc.arg(window_end)::timestamptz
GROUP BY c.id, o.ref_id, c.author_id, c.body, c.created_at
ORDER BY count(*) DESC, c.created_at, c.id
LIMIT 1;

-- name: DeleteUserItemReactions :exec
DELETE FROM feed_reactions WHERE user_id = sqlc.arg(user_id);

-- name: DeleteUserItemLikes :exec
DELETE FROM feed_likes WHERE user_id = sqlc.arg(user_id);

-- name: DeleteUserCommentReactions :exec
DELETE FROM comment_reactions WHERE user_id = sqlc.arg(user_id);
