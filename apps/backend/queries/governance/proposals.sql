-- name: InsertProposal :execrows
WITH proposal AS (
  INSERT INTO proposals (
    id, cabal_id, proposer_id, kind, symbol, mint, usdc_micros, token_amount, thesis,
    quote_out_amount, status, expires_at, created_at, updated_at
  )
  VALUES (
    @id, @cabal_id, @proposer_id, @kind, @symbol, @mint, @usdc_micros, @token_amount, @thesis,
    @quote_out_amount, 'open', @expires_at, @created_at, @created_at
  )
  RETURNING id
)
INSERT INTO proposal_voters (proposal_id, voter_id)
SELECT proposal.id, voter
FROM proposal, unnest(@voter_ids::uuid[]) AS voter;

-- name: UpsertBallot :exec
INSERT INTO votes (proposal_id, voter_id, choice, cast_at)
VALUES (@proposal_id, @voter_id, @choice, @cast_at)
ON CONFLICT (proposal_id, voter_id) DO UPDATE SET choice = excluded.choice, cast_at = excluded.cast_at;

-- name: CountBallots :one
SELECT
  count(*)::int AS voters,
  (count(*) FILTER (WHERE votes.choice = 'yes'))::int AS yes,
  (count(*) FILTER (WHERE votes.choice = 'no'))::int AS no
FROM proposal_voters
LEFT JOIN votes USING (proposal_id, voter_id)
WHERE proposal_voters.proposal_id = @proposal_id;

-- name: Transition :execrows
UPDATE proposals
SET status = @to_status::text,
  status_reason = CASE WHEN @to_status::text = 'execution_blocked' THEN sqlc.narg(reason)::text END,
  void_reason = CASE WHEN @to_status::text = 'voided' THEN sqlc.narg(reason)::text END,
  updated_at = @at::timestamptz
WHERE id = @id AND status = @from_status::text;

-- name: StatusByID :one
SELECT status FROM proposals WHERE id = @id;
