-- name: StartWindDown :execrows
INSERT INTO cabal_winddowns (cabal_id, status, started_at)
VALUES (sqlc.arg(cabal_id)::uuid, 'running', sqlc.arg(at)::timestamptz)
ON CONFLICT (cabal_id) DO NOTHING;

-- name: RunningWindDowns :many
SELECT w.cabal_id, w.attempts, w.started_at,
  coalesce(
    (SELECT max(j.created_at) FROM cash_out_jobs j WHERE j.cabal_id = w.cabal_id AND j.cause = 'wind_down'),
    w.started_at
  )::timestamptz AS last_issued_at
FROM cabal_winddowns w
WHERE w.status = 'running'
ORDER BY w.started_at, w.cabal_id;

-- name: WindDownHolders :many
SELECT user_id FROM user_positions
WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND share_units > 0
ORDER BY user_id;

-- name: BumpWindDownAttempts :execrows
UPDATE cabal_winddowns SET attempts = attempts + 1 WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND status = 'running';

-- name: ReopenWindDown :execrows
UPDATE cabal_winddowns
SET status = 'running', attempts = 0, started_at = sqlc.arg(at)::timestamptz, completed_at = NULL
WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND status = 'completed';

-- name: ResetWindDownAttempts :execrows
UPDATE cabal_winddowns SET attempts = 0 WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND status = 'running';

-- name: CompleteWindDown :one
UPDATE cabal_winddowns SET status = 'completed', completed_at = sqlc.arg(at)::timestamptz
WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND status = 'running'
  AND NOT EXISTS (SELECT 1 FROM user_positions WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND share_units > 0)
  AND NOT EXISTS (
    SELECT 1 FROM cash_out_jobs
    WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND status NOT IN ('completed', 'partial', 'failed')
  )
  AND NOT EXISTS (
    SELECT 1 FROM fund_transfers
    WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND status IN ('created', 'submitted', 'landed')
  )
RETURNING
  (SELECT count(DISTINCT user_id) FROM cash_out_jobs
    WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND cause = 'wind_down' AND status IN ('completed', 'partial')
      AND payout_micros > 0)::bigint
    AS members_paid,
  (SELECT coalesce(sum(payout_micros), 0) FROM cash_out_jobs
    WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND cause = 'wind_down' AND status IN ('completed', 'partial'))::bigint
    AS returned_micros;
