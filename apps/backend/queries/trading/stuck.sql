-- name: StuckSwaps :many
WITH stuck AS (
  SELECT u.id, u.since
  FROM (
    SELECT s.id, s.created_at AS since FROM swaps AS s
    WHERE s.status = 'created' AND s.created_at < @cutoff::timestamptz
    UNION ALL
    SELECT s.id, s.submitted_at AS since FROM swaps AS s
    WHERE s.status = 'submitted' AND s.submitted_at < @cutoff::timestamptz
  ) AS u
  ORDER BY u.since, u.id
  LIMIT @row_limit::bigint
)
SELECT v.id, v.cabal_id, v.source_kind, v.source_id, v.action, v.symbol, v.in_amount, v.out_decimals, v.out_amount,
  v.status, v.failure_code, v.tx_signature, v.created_at, v.confirmed_at, v.retryable
FROM stuck
JOIN swap_views AS v ON v.id = stuck.id
ORDER BY stuck.since, v.id;

-- name: CountStuckSwaps :one
SELECT ((
  SELECT count(*) FROM swaps WHERE status = 'created' AND created_at < @cutoff::timestamptz
) + (
  SELECT count(*) FROM swaps WHERE status = 'submitted' AND submitted_at < @cutoff::timestamptz
))::bigint AS stuck;

-- name: ExecuteRequestID :one
SELECT coalesce(execute_request_id, '')::text AS execute_request_id FROM swaps WHERE id = @id;
