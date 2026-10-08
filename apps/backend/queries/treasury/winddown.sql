-- name: StartWindDown :execrows
INSERT INTO cabal_winddowns (cabal_id, status, started_at)
VALUES (sqlc.arg(cabal_id)::uuid, 'running', sqlc.arg(at)::timestamptz)
ON CONFLICT (cabal_id) DO NOTHING;

-- name: RunningWindDowns :many
SELECT cabal_id, attempts, started_at FROM cabal_winddowns WHERE status = 'running' ORDER BY started_at, cabal_id;

-- name: WindDownHolders :many
SELECT user_id FROM user_positions
WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND share_units > 0
ORDER BY user_id;

-- name: BumpWindDownAttempts :execrows
UPDATE cabal_winddowns SET attempts = attempts + 1 WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND status = 'running';

-- name: CompleteWindDown :one
UPDATE cabal_winddowns SET status = 'completed', completed_at = sqlc.arg(at)::timestamptz
WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND status = 'running'
  AND NOT EXISTS (SELECT 1 FROM user_positions WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND share_units > 0)
  AND NOT EXISTS (
    SELECT 1 FROM cash_out_jobs
    WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND cause = 'wind_down' AND status NOT IN ('completed', 'partial', 'failed')
  )
RETURNING
  (SELECT count(DISTINCT user_id) FROM cash_out_jobs
    WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND cause = 'wind_down' AND status IN ('completed', 'partial')
      AND payout_micros > 0)::bigint
    AS members_paid,
  (SELECT coalesce(sum(payout_micros), 0) FROM cash_out_jobs
    WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND cause = 'wind_down' AND status IN ('completed', 'partial'))::bigint
    AS returned_micros;
