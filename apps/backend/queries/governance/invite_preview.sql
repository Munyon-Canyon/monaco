-- name: NewestOpenProposalForCabal :one
SELECT
  p.id, p.kind, p.symbol, p.mint, p.usdc_micros, p.proposer_id, p.thesis, p.expires_at,
  ballots.yes_count, ballots.no_count, ballots.voted_count, electorate.voter_count
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
  SELECT count(*)::int AS voter_count FROM proposal_voters AS pv WHERE pv.proposal_id = p.id
) AS electorate
WHERE p.cabal_id = @cabal_id AND p.status = 'open'
ORDER BY p.created_at DESC, p.id DESC
LIMIT 1;

-- name: CabalTurnoutSince :one
SELECT
  (SELECT count(*) FROM votes AS v JOIN proposals AS p ON p.id = v.proposal_id
    WHERE p.cabal_id = @cabal_id AND p.created_at >= @since)::int AS voted_count,
  (SELECT count(*) FROM proposal_voters AS pv JOIN proposals AS p ON p.id = pv.proposal_id
    WHERE p.cabal_id = @cabal_id AND p.created_at >= @since)::int AS eligible_count;
