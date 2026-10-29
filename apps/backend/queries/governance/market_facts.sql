-- name: OpenFactsForVoter :many
SELECT DISTINCT ON (p.symbol)
  p.symbol, p.cabal_id, (count(*) OVER (PARTITION BY p.symbol))::int AS open_count
FROM unnest(@symbols::text[]) AS wanted(symbol)
JOIN proposals AS p ON p.symbol = wanted.symbol
JOIN proposal_voters AS pv ON pv.proposal_id = p.id AND pv.voter_id = @caller_id
WHERE p.status = 'open' AND p.cabal_id = ANY(@cabal_ids::uuid[])
ORDER BY p.symbol, p.expires_at, p.id;

-- name: CalledFacts :many
SELECT DISTINCT ON (p.symbol) p.symbol, p.proposer_id
FROM unnest(@symbols::text[]) AS wanted(symbol)
JOIN proposals AS p ON p.symbol = wanted.symbol
WHERE p.status IN ('passed', 'executed') AND p.closed_at >= @since::timestamptz AND p.cabal_id = ANY(@cabal_ids::uuid[])
ORDER BY p.symbol, p.closed_at DESC, p.id DESC;
