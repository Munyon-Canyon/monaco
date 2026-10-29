-- name: VotesBy :many
SELECT
  p.id AS proposal_id, p.cabal_id, p.kind, p.symbol, p.mint, coalesce(p.usdc_micros, -1)::bigint AS usdc_micros, v.choice,
  v.cast_at
FROM votes AS v
JOIN proposals AS p ON p.id = v.proposal_id
WHERE v.voter_id = @voter_id
  AND (NOT @has_cursor::bool OR (v.cast_at, v.proposal_id) < (@cursor_at::timestamptz, @cursor_id::uuid))
ORDER BY v.cast_at DESC, v.proposal_id DESC
LIMIT @row_limit;

-- name: ProposalsBy :many
SELECT
  p.id, p.cabal_id, p.kind, p.symbol, p.mint, coalesce(p.usdc_micros, -1)::bigint AS usdc_micros, p.status,
  p.created_at,
  (SELECT count(*) FROM votes AS v WHERE v.proposal_id = p.id AND v.choice = 'yes')::int AS yes_count,
  (SELECT count(*) FROM votes AS v WHERE v.proposal_id = p.id AND v.choice = 'no')::int AS no_count
FROM proposals AS p
WHERE p.proposer_id = @proposer_id
  AND (NOT @has_cursor::bool OR (p.created_at, p.id) < (@cursor_at::timestamptz, @cursor_id::uuid))
ORDER BY p.created_at DESC, p.id DESC
LIMIT @row_limit;

-- name: ProposalSubjects :many
SELECT id, cabal_id, kind, symbol, coalesce(usdc_micros, -1)::bigint AS usdc_micros
FROM proposals
WHERE id = ANY(@proposal_ids::uuid[]);
