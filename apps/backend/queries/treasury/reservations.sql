-- name: CashOutReservations :many
SELECT cabal_id, sum(payout_micros)::text AS reserved_micros
FROM cash_out_jobs
WHERE status IN ('started', 'selling', 'paying')
GROUP BY cabal_id;

-- name: CashOutReservationsAt :many
SELECT cabal_id, sum(payout_micros)::text AS reserved_micros
FROM cash_out_jobs
WHERE created_at <= sqlc.arg(at)::timestamptz
  AND (status IN ('started', 'selling', 'paying') OR updated_at > sqlc.arg(at)::timestamptz)
GROUP BY cabal_id;
