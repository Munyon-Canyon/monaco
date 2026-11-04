-- name: ListBallots :many
SELECT pv.voter_id, v.choice, v.cast_at
FROM proposal_voters AS pv
LEFT JOIN votes AS v ON v.proposal_id = pv.proposal_id AND v.voter_id = pv.voter_id
WHERE pv.proposal_id = @proposal_id
ORDER BY pv.voter_id;

-- name: MyBallots :many
SELECT pv.proposal_id, coalesce(v.choice, '')::text AS choice,
  (p.status = 'open' AND p.expires_at > now())::boolean AS can_vote
FROM proposal_voters AS pv
JOIN proposals AS p ON p.id = pv.proposal_id
LEFT JOIN votes AS v ON v.proposal_id = pv.proposal_id AND v.voter_id = pv.voter_id
WHERE pv.voter_id = @viewer AND pv.proposal_id = ANY(@proposal_ids::uuid[]);
