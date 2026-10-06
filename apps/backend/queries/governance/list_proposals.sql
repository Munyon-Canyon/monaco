-- name: ListProposals :many
SELECT
  p.id, p.cabal_id, p.proposer_id, p.kind, p.symbol, p.usdc_micros, p.token_amount, p.quote_out_amount,
  p.thesis, p.status, p.status_reason, p.expires_at, p.created_at,
  coalesce(mine.choice, '')::text AS my_ballot
FROM proposals AS p
LEFT JOIN votes AS mine ON mine.proposal_id = p.id AND mine.voter_id = @caller_id
WHERE p.cabal_id = @cabal_id
  AND (@filter::text = 'all' OR (p.status = 'open') = (@filter::text = 'open'))
  AND (NOT @has_cursor::bool OR (p.created_at, p.id) < (@cursor_created_at::timestamptz, @cursor_id::uuid))
ORDER BY p.created_at DESC, p.id DESC
LIMIT @row_limit;

-- name: TallyProposals :many
SELECT
  proposal_voters.proposal_id,
  count(*)::int AS voters,
  (count(*) FILTER (WHERE votes.choice = 'yes'))::int AS yes,
  (count(*) FILTER (WHERE votes.choice = 'no'))::int AS no,
  bool_or(proposal_voters.voter_id = @caller_id)::bool AS caller_votes
FROM proposal_voters
LEFT JOIN votes USING (proposal_id, voter_id)
WHERE proposal_voters.proposal_id = ANY(@proposal_ids::uuid[])
GROUP BY proposal_voters.proposal_id;
