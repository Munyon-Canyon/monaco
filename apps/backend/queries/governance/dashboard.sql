-- name: ProposalCounts :many
SELECT (date_trunc(@bucket::text, e.created_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC')::timestamptz AS bucket_start,
  count(*) FILTER (WHERE e.type = 'proposal.created')::bigint AS created,
  count(*) FILTER (WHERE e.type = 'proposal.passed')::bigint AS passed,
  count(*) FILTER (WHERE e.type = 'proposal.failed')::bigint AS failed,
  count(*) FILTER (WHERE e.type = 'proposal.expired')::bigint AS expired,
  count(*) FILTER (WHERE e.type = 'proposal.execution_blocked')::bigint AS execution_blocked
FROM events AS e
WHERE e.type IN (
    'proposal.created', 'proposal.passed', 'proposal.failed', 'proposal.expired', 'proposal.execution_blocked'
  )
  AND e.created_at >= @from_at::timestamptz AND e.created_at < @to_at::timestamptz
GROUP BY 1
ORDER BY 1;

-- name: MedianTimeToPass :one
SELECT count(*)::bigint AS passed,
  coalesce(round(percentile_cont(0.5) WITHIN GROUP (
    ORDER BY extract(epoch FROM (passed.created_at - created.created_at))::float8
  )), 0)::bigint AS median_seconds
FROM events AS passed
JOIN events AS created ON created.aggregate_id = passed.aggregate_id AND created.type = 'proposal.created'
WHERE passed.type = 'proposal.passed'
  AND passed.created_at >= @from_at::timestamptz AND passed.created_at < @to_at::timestamptz;

-- name: VoteParticipation :many
SELECT p.cabal_id,
  count(DISTINCT p.id)::bigint AS proposals,
  count(pv.voter_id)::bigint AS eligible,
  count(v.voter_id)::bigint AS voted
FROM proposals AS p
JOIN proposal_voters AS pv ON pv.proposal_id = p.id
LEFT JOIN votes AS v ON v.proposal_id = pv.proposal_id AND v.voter_id = pv.voter_id
WHERE p.created_at >= @from_at::timestamptz AND p.created_at < @to_at::timestamptz
GROUP BY p.cabal_id
ORDER BY p.cabal_id;

-- name: OpenProposalCount :one
SELECT count(*)::bigint FROM proposals WHERE status = 'open';
