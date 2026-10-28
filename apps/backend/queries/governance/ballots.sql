-- name: ListBallots :many
SELECT pv.voter_id, v.choice, v.cast_at
FROM proposal_voters AS pv
LEFT JOIN votes AS v ON v.proposal_id = pv.proposal_id AND v.voter_id = pv.voter_id
WHERE pv.proposal_id = @proposal_id
ORDER BY pv.voter_id;
