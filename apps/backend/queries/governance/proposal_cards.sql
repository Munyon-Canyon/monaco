-- name: ProposalCards :many
SELECT
  p.id, p.cabal_id, p.proposer_id, p.kind, p.symbol, p.mint, p.usdc_micros, p.token_amount, p.threshold,
  p.status, p.decided_by, p.expires_at, p.created_at,
  ballots.yes_count, ballots.no_count, ballots.voted_count, electorate.voter_count,
  mine.choice AS your_choice
FROM proposals AS p
CROSS JOIN LATERAL (
  SELECT
    (count(*) FILTER (WHERE v.choice = 'yes'))::int AS yes_count,
    (count(*) FILTER (WHERE v.choice = 'no'))::int AS no_count,
    count(*)::int AS voted_count
  FROM votes AS v
  WHERE v.proposal_id = p.id
) AS ballots
CROSS JOIN LATERAL (
  SELECT count(*)::int AS voter_count FROM proposal_voters AS all_voters WHERE all_voters.proposal_id = p.id
) AS electorate
LEFT JOIN votes AS mine ON mine.proposal_id = p.id AND mine.voter_id = @viewer_id
WHERE p.id = ANY(@proposal_ids::uuid[])
ORDER BY p.created_at, p.id;
