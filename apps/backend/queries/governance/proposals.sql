-- name: InsertProposal :execrows
WITH proposal AS (
  INSERT INTO proposals (
    id, cabal_id, proposer_id, kind, symbol, mint, usdc_micros, token_amount, thesis,
    quote_out_amount, threshold, status, expires_at, created_at, updated_at
  )
  VALUES (
    @id, @cabal_id, @proposer_id, @kind, @symbol, @mint, @usdc_micros, @token_amount, @thesis,
    @quote_out_amount, @threshold, 'open', @expires_at, @created_at, @created_at
  )
  RETURNING id
)
INSERT INTO proposal_voters (proposal_id, voter_id)
SELECT proposal.id, voter
FROM proposal, unnest(@voter_ids::uuid[]) AS voter;

-- name: OpenProposal :execrows
WITH proposal AS (
  INSERT INTO proposals (
    id, cabal_id, proposer_id, kind, symbol, mint, usdc_micros, token_amount, thesis,
    quote_out_amount, threshold, status, expires_at, created_at, updated_at
  )
  VALUES (
    sqlc.arg(id), sqlc.arg(cabal_id), sqlc.arg(proposer_id), sqlc.arg(kind), sqlc.arg(symbol), sqlc.arg(mint),
    NULLIF(sqlc.arg(usdc_micros)::text, '0')::bigint, NULLIF(sqlc.arg(token_amount)::text, '0')::bigint,
    NULLIF(sqlc.arg(thesis)::text, ''), sqlc.arg(quote_out_amount)::text::bigint, sqlc.arg(threshold), 'open', sqlc.arg(expires_at),
    sqlc.arg(created_at), sqlc.arg(created_at)
  )
  RETURNING id
)
INSERT INTO proposal_voters (proposal_id, voter_id)
SELECT proposal.id, voter
FROM proposal, unnest(sqlc.arg(voter_ids)::uuid[]) AS voter;

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

-- name: CabalOfProposal :one
SELECT cabal_id FROM proposals WHERE id = @id;

-- name: ProposerOfProposal :one
SELECT proposer_id FROM proposals WHERE id = @id;

-- name: VotersOfProposal :many
SELECT voter_id FROM votes WHERE proposal_id = @proposal_id ORDER BY cast_at, voter_id;

-- name: LockProposal :one
SELECT
  p.id, p.cabal_id, p.proposer_id, p.kind, p.symbol, p.mint, p.usdc_micros, p.token_amount, p.quote_out_amount,
  p.status, p.threshold,
  EXISTS (
    SELECT 1 FROM proposal_voters AS v WHERE v.proposal_id = p.id AND v.voter_id = @voter_id
  ) AS is_voter
FROM proposals AS p
WHERE p.id = @id
FOR UPDATE OF p;

-- name: CastBallot :one
WITH ballot AS (
  INSERT INTO votes (proposal_id, voter_id, choice, cast_at)
  VALUES (@proposal_id, @voter_id, @choice, @cast_at)
  ON CONFLICT (proposal_id, voter_id) DO UPDATE SET choice = excluded.choice, cast_at = excluded.cast_at
  RETURNING choice
),
ballots AS (
  SELECT votes.choice FROM votes WHERE votes.proposal_id = @proposal_id AND votes.voter_id <> @voter_id
  UNION ALL
  SELECT ballot.choice FROM ballot
)
SELECT
  (SELECT count(*) FROM proposal_voters WHERE proposal_voters.proposal_id = @proposal_id)::int AS voters,
  (count(*) FILTER (WHERE ballots.choice = 'yes'))::int AS yes,
  (count(*) FILTER (WHERE ballots.choice = 'no'))::int AS no
FROM ballots;

-- name: DueForExpiry :many
SELECT id, cabal_id
FROM proposals
WHERE status = 'open' AND expires_at <= @now
ORDER BY expires_at, id
LIMIT @batch;

-- name: LockForWithdraw :one
SELECT
  p.id, p.cabal_id, p.proposer_id, p.status,
  EXISTS (
    SELECT 1 FROM votes AS v WHERE v.proposal_id = p.id AND v.voter_id <> p.proposer_id
  ) AS others_voted
FROM proposals AS p
WHERE p.id = @id
FOR UPDATE OF p;

-- name: Void :one
UPDATE proposals
SET status = @to_status::text, void_reason = @reason::text, updated_at = @at::timestamptz
WHERE id = @id AND status = ANY(@from_statuses::text[])
RETURNING cabal_id;
