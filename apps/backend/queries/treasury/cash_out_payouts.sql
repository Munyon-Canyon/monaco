-- name: CashOutPayoutJob :one
SELECT j.id, j.cabal_id, j.user_id, j.share_units::text AS share_units, j.returned_units::text AS returned_units,
  j.payout_micros::text AS payout_micros, j.slice_micros::text AS slice_micros, j.sell_usdc_micros > 0 AS selling,
  j.status, coalesce(p.attempt, 0)::smallint AS attempt, coalesce(p.signature, '')::text AS signature,
  coalesce(p.signed_tx, '\x'::bytea)::bytea AS signed_tx, coalesce(p.status, '')::text AS payout_status,
  coalesce(p.last_valid_block_height, 0)::bigint AS last_valid_block_height
FROM cash_out_jobs AS j
LEFT JOIN LATERAL (
  SELECT attempt, signature, signed_tx, status, last_valid_block_height FROM cash_out_payouts
  WHERE job_id = j.id ORDER BY attempt DESC LIMIT 1
) AS p ON true
WHERE j.id = sqlc.arg(id)::uuid;

-- name: CashOutPayoutsDue :many
SELECT j.id
FROM cash_out_jobs AS j
LEFT JOIN LATERAL (
  SELECT status, created_at FROM cash_out_payouts WHERE job_id = j.id ORDER BY attempt DESC LIMIT 1
) AS p ON true
WHERE (j.status = 'started' AND j.sell_usdc_micros = 0 AND j.created_at < sqlc.arg(stale_before)::timestamptz)
  OR (j.status = 'paying' AND (p.status IS NULL OR p.status IN ('expired', 'failed')
    OR p.created_at < sqlc.arg(stale_before)::timestamptz))
ORDER BY j.updated_at, j.id
LIMIT sqlc.arg(max_rows)::integer;

-- name: InsertCashOutPayout :execrows
INSERT INTO cash_out_payouts (job_id, attempt, signature, signed_tx, last_valid_block_height, status, created_at)
VALUES (sqlc.arg(job_id)::uuid, sqlc.arg(attempt)::smallint, sqlc.arg(signature)::text,
  sqlc.arg(signed_tx)::bytea, sqlc.arg(last_valid_block_height)::text::bigint, 'signed', sqlc.arg(at)::timestamptz)
ON CONFLICT DO NOTHING;

-- name: MoveCashOutPayout :execrows
UPDATE cash_out_payouts SET status = sqlc.arg(to_status)::text
WHERE job_id = sqlc.arg(job_id)::uuid AND attempt = sqlc.arg(attempt)::smallint
  AND status = ANY(sqlc.arg(from_statuses)::text[]);

-- name: EndCashOutJob :execrows
UPDATE cash_out_jobs
SET status = sqlc.arg(to_status)::text, result_code = coalesce(NULLIF(sqlc.arg(result_code)::text, ''), result_code),
  updated_at = sqlc.arg(at)::timestamptz
WHERE id = sqlc.arg(id)::uuid AND status = sqlc.arg(from_status)::text;

-- name: FailUnpairedUserTransfer :execrows
UPDATE user_txns SET status = 'failed'
WHERE transfer_id = sqlc.arg(transfer_id)::uuid AND status = 'pending'
  AND NOT EXISTS (SELECT 1 FROM cabal_txns WHERE transfer_id = sqlc.arg(transfer_id)::uuid);
