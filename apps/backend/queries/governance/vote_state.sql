-- name: GetVoteState :one
SELECT
  p.cabal_id, p.kind, p.symbol, p.mint, coalesce(p.usdc_micros, p.quote_out_amount, 0)::bigint AS usdc_micros,
  p.status, p.threshold, p.created_at, p.expires_at,
  (SELECT count(*) FROM votes AS v WHERE v.proposal_id = p.id AND v.choice = 'yes')::int AS yes,
  (SELECT count(*) FROM votes AS v WHERE v.proposal_id = p.id AND v.choice = 'no')::int AS no,
  (SELECT count(*) FROM proposal_voters AS pv WHERE pv.proposal_id = p.id)::int AS eligible
FROM proposals AS p
WHERE p.id = @id;

-- name: DueForReminder :many
SELECT id, due_at FROM (
  SELECT id, (created_at + (expires_at - created_at) / 3)::timestamptz AS due_at, expires_at
  FROM proposals
  WHERE status = 'open' AND kind <> 'rule'
) AS due
WHERE (due_at, id) > (@after_due_at::timestamptz, @after_id::uuid)
  AND due_at <= @now::timestamptz AND @now::timestamptz < expires_at
ORDER BY due_at, id
LIMIT @batch;

-- name: NonVotersOfProposal :many
SELECT pv.voter_id
FROM proposal_voters AS pv
LEFT JOIN votes AS v ON v.proposal_id = pv.proposal_id AND v.voter_id = pv.voter_id
WHERE pv.proposal_id = @proposal_id AND v.voter_id IS NULL
ORDER BY pv.voter_id;
