-- name: CashOutShares :one
SELECT coalesce((SELECT share_units FROM user_positions
  WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND user_id = sqlc.arg(user_id)::uuid), 0)::text AS shares,
  coalesce((SELECT sum(share_units) FROM user_positions WHERE cabal_id = sqlc.arg(cabal_id)::uuid), 0)::text AS total;

-- name: CashOutLiveJob :one
SELECT EXISTS(SELECT 1 FROM cash_out_jobs
  WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND user_id = sqlc.arg(user_id)::uuid
    AND status NOT IN ('completed', 'partial', 'failed'));

-- name: CashOutShortfall :one
SELECT greatest(sqlc.arg(payout_micros)::text::numeric - greatest(coalesce((SELECT units FROM cabal_positions
  WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND asset = sqlc.arg(asset)::text), 0) - coalesce((SELECT
    sum(CASE WHEN j.status = 'paying' THEN j.payout_micros ELSE least(j.payout_micros,
      j.payout_micros - j.sell_usdc_micros + coalesce((SELECT sum(s.usdc_out_micros) FROM cash_out_sells AS s
        WHERE s.job_id = j.id), 0)) END)
  FROM cash_out_jobs AS j
  WHERE j.cabal_id = sqlc.arg(cabal_id)::uuid AND j.status IN ('started', 'selling', 'paying')), 0), 0),
  0)::text;

-- name: InsertCashOutJob :exec
INSERT INTO cash_out_jobs
  (id, cabal_id, user_id, share_units, payout_micros, slice_micros, sell_usdc_micros, status, created_at, updated_at)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(cabal_id)::uuid, sqlc.arg(user_id)::uuid,
  sqlc.arg(share_units)::text::numeric, sqlc.arg(payout_micros)::text::numeric, sqlc.arg(payout_micros)::text::numeric,
  sqlc.arg(sell_usdc_micros)::text::numeric, 'started', sqlc.arg(at)::timestamptz, sqlc.arg(at)::timestamptz);

-- name: CashOutJobByID :one
SELECT id, cabal_id, user_id, share_units::text AS share_units, payout_micros::text AS payout_micros,
  sell_usdc_micros::text AS sell_usdc_micros, status, result_code, created_at, updated_at
FROM cash_out_jobs
WHERE id = sqlc.arg(id)::uuid AND cabal_id = sqlc.arg(cabal_id)::uuid AND user_id = sqlc.arg(user_id)::uuid;

-- name: LockCashOutJob :one
SELECT status, user_id, share_units::text AS share_units, slice_micros::text AS slice_micros
FROM cash_out_jobs
WHERE id = sqlc.arg(id)::uuid AND cabal_id = sqlc.arg(cabal_id)::uuid
FOR UPDATE;

-- name: MoveCashOutJob :execrows
UPDATE cash_out_jobs SET status = sqlc.arg(to_status)::text, updated_at = sqlc.arg(at)::timestamptz
WHERE id = sqlc.arg(id)::uuid AND status = sqlc.arg(from_status)::text;

-- name: InsertCashOutSell :exec
INSERT INTO cash_out_sells (swap_id, job_id, batch_size, status, usdc_out_micros, created_at)
VALUES (sqlc.arg(swap_id)::uuid, sqlc.arg(job_id)::uuid, sqlc.arg(batch_size)::bigint, sqlc.arg(status)::text,
  sqlc.arg(usdc_out_micros)::text::numeric, sqlc.arg(at)::timestamptz)
ON CONFLICT (swap_id) DO NOTHING;

-- name: CashOutSaleTally :one
SELECT count(s.swap_id)::integer AS results, coalesce(max(s.batch_size), 0)::integer AS batch_size,
  least(j.slice_micros, j.payout_micros - j.sell_usdc_micros + coalesce(sum(s.usdc_out_micros), 0))::text AS paid
FROM cash_out_jobs AS j
LEFT JOIN cash_out_sells AS s ON s.job_id = j.id
WHERE j.id = sqlc.arg(id)::uuid
GROUP BY j.id;

-- name: SettleCashOutSale :one
WITH settled AS (
  UPDATE cash_out_jobs SET status = sqlc.arg(to_status)::text,
    payout_micros = sqlc.arg(payout_micros)::text::numeric,
    returned_units = sqlc.arg(returned_units)::text::numeric, result_code = NULLIF(sqlc.arg(result_code)::text, ''),
    updated_at = sqlc.arg(at)::timestamptz
  WHERE id = sqlc.arg(id)::uuid AND status = 'selling'
  RETURNING id, status
), failed AS (
  UPDATE user_txns SET status = 'failed'
  WHERE transfer_id IN (SELECT id FROM settled WHERE status = 'failed') AND kind = 'cash_out' AND status = 'pending'
  RETURNING 1
)
SELECT (SELECT count(*) FROM settled)::bigint AS settled, (SELECT count(*) FROM failed)::bigint AS failed;
