-- name: ListBallots :many
SELECT proposal_voters.voter_id, votes.choice, votes.cast_at
FROM proposal_voters
LEFT JOIN votes
  ON votes.proposal_id = proposal_voters.proposal_id AND votes.voter_id = proposal_voters.voter_id
WHERE proposal_voters.proposal_id = @proposal_id
ORDER BY proposal_voters.voter_id;
