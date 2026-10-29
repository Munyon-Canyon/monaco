-- name: ProposalReplyCounts :many
SELECT o.ref_id AS proposal_id, count(c.id)::int AS reply_count
FROM feed_objects AS o
JOIN feed_comments AS c ON c.feed_object_id = o.id AND c.deleted_at IS NULL AND c.author_kind = 'user'
WHERE o.ref_type = 'proposals' AND o.kind = 'proposal' AND o.ref_id = ANY(sqlc.arg(proposal_ids)::uuid[])
GROUP BY o.ref_id;
