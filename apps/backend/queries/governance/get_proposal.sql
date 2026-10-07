-- name: GetProposal :one
SELECT
  p.id, p.cabal_id, p.proposer_id, p.kind, p.symbol, p.usdc_micros, p.token_amount, p.quote_out_amount,
  p.thesis, p.threshold, p.status, p.status_reason, p.expires_at, p.created_at,
  coalesce(mine.choice, '')::text AS my_ballot
FROM proposals AS p
LEFT JOIN votes AS mine ON mine.proposal_id = p.id AND mine.voter_id = @caller_id
WHERE p.id = @id;

-- name: ProposalVoters :many
SELECT proposal_voters.voter_id, votes.choice, votes.cast_at
FROM proposal_voters
LEFT JOIN votes USING (proposal_id, voter_id)
WHERE proposal_voters.proposal_id = @proposal_id
ORDER BY proposal_voters.voter_id;
