-- name: GetProposal :one
SELECT
  p.id, p.cabal_id, p.proposer_id, p.kind, p.symbol, p.usdc_micros, p.token_amount, p.quote_out_amount,
  p.thesis, p.threshold, p.status, p.status_reason, p.expires_at, p.created_at, p.pot_value_micros, p.closed_at,
  p.passed_price_micros, p.decided_by, p.sides_after_close,
  coalesce(mine.choice, '')::text AS my_ballot, p.price_comparison, coalesce(p.network, '')::text AS network, p.mint
FROM proposals AS p
LEFT JOIN votes AS mine ON mine.proposal_id = p.id AND mine.voter_id = @caller_id
WHERE p.id = @id;

-- name: ProposalVoters :many
SELECT proposal_voters.voter_id, votes.choice, votes.cast_at
FROM proposal_voters
LEFT JOIN votes USING (proposal_id, voter_id)
WHERE proposal_voters.proposal_id = @proposal_id
ORDER BY proposal_voters.voter_id;

-- name: BallotOrder :one
WITH ranked AS (
  SELECT
    votes.voter_id,
    votes.choice,
    row_number() OVER (ORDER BY votes.first_cast_at, votes.voter_id)::int AS order_all,
    row_number() OVER (PARTITION BY votes.choice ORDER BY votes.cast_at, votes.voter_id)::int AS order_side
  FROM votes
  WHERE votes.proposal_id = @proposal_id
)
SELECT
  ranked.order_all AS "order",
  (CASE WHEN ranked.choice = 'yes' THEN ranked.order_side ELSE 0 END)::int AS yes_order,
  (CASE WHEN ranked.choice = 'no' THEN ranked.order_side ELSE 0 END)::int AS no_order,
  (SELECT count(*) FROM proposal_voters WHERE proposal_voters.proposal_id = @proposal_id)::int AS voters
FROM ranked
WHERE ranked.voter_id = @voter_id::uuid;
