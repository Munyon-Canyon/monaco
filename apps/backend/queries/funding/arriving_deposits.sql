-- name: UserHasArrivingDeposit :one
SELECT (
  EXISTS (
    SELECT 1 FROM onramp_sessions s
    WHERE s.user_id = sqlc.arg(member)::uuid AND s.status IN ('submitted', 'confirmed')
      AND s.completed_at >= sqlc.arg(since)::timestamptz
  )
  OR EXISTS (
    SELECT 1 FROM onramp_sessions s
    WHERE s.user_id = sqlc.arg(member)::uuid AND s.status = 'opened'
      AND s.opened_at > sqlc.arg(opened_since)::timestamptz
  )
  OR EXISTS (
    SELECT 1 FROM evm_deposit_forwards f WHERE f.user_id = sqlc.arg(member)::uuid AND f.status = 'pending'
  )
)::boolean AS arriving;
