-- name: PendingVotes :many
SELECT
  p.id, p.cabal_id, p.proposer_id, p.kind, p.symbol, p.mint, p.usdc_micros, p.token_amount, p.threshold,
  p.expires_at, p.created_at,
  ballots.yes_count, ballots.no_count, ballots.voted_count, electorate.voter_count
FROM proposal_voters AS pv
JOIN proposals AS p ON p.id = pv.proposal_id
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
WHERE pv.voter_id = @voter_id
  AND p.status = 'open'
  AND NOT EXISTS (SELECT 1 FROM votes AS v WHERE v.proposal_id = p.id AND v.voter_id = @voter_id)
ORDER BY p.expires_at, p.id
LIMIT 50;
