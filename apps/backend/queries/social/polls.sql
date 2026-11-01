-- name: InsertCommentPollOptions :exec
INSERT INTO comment_poll_options (comment_id, position, label)
SELECT sqlc.arg(comment_id)::uuid, o.position::smallint, o.label
FROM (
  SELECT unnest(sqlc.arg(positions)::int[]) AS position, unnest(sqlc.arg(labels)::text[]) AS label
) AS o;

-- name: ListCommentPolls :many
SELECT o.comment_id, o.position, o.label,
  (SELECT count(*) FROM comment_poll_votes v WHERE v.comment_id = o.comment_id AND v.position = o.position)::int AS votes,
  EXISTS (
    SELECT 1 FROM comment_poll_votes m
    WHERE m.comment_id = o.comment_id AND m.position = o.position AND m.user_id = sqlc.arg(viewer)::uuid
  )::bool AS mine
FROM comment_poll_options o
WHERE o.comment_id = ANY(sqlc.arg(comment_ids)::uuid[])
ORDER BY o.comment_id, o.position;

-- name: GetCommentPoll :one
SELECT c.feed_object_id, (c.deleted_at IS NOT NULL)::bool AS deleted, a.payload
FROM feed_comments c
JOIN comment_attachments a ON a.comment_id = c.id AND a.kind = 'poll'
WHERE c.id = sqlc.arg(comment_id);

-- name: UpsertCommentPollVote :execrows
INSERT INTO comment_poll_votes (comment_id, user_id, position, created_at, updated_at)
SELECT o.comment_id, sqlc.arg(user_id)::uuid, o.position, sqlc.arg(at)::timestamptz, sqlc.arg(at)::timestamptz
FROM comment_poll_options o
WHERE o.comment_id = sqlc.arg(comment_id) AND o.position = sqlc.arg(position)::smallint
ON CONFLICT (comment_id, user_id) DO UPDATE SET position = EXCLUDED.position, updated_at = EXCLUDED.updated_at;

-- name: DeleteCommentPollVote :execrows
DELETE FROM comment_poll_votes WHERE comment_id = sqlc.arg(comment_id) AND user_id = sqlc.arg(user_id);

-- name: DeleteUserPollVotes :exec
DELETE FROM comment_poll_votes WHERE user_id = sqlc.arg(user_id);
