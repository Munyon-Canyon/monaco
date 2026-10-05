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
    sum(CASE WHEN status = 'paying' THEN payout_micros ELSE payout_micros - sell_usdc_micros END)
  FROM cash_out_jobs WHERE cabal_id = sqlc.arg(cabal_id)::uuid AND status IN ('started', 'selling', 'paying')), 0), 0),
  0)::text;

-- name: InsertCashOutJob :exec
INSERT INTO cash_out_jobs
  (id, cabal_id, user_id, share_units, payout_micros, sell_usdc_micros, status, created_at, updated_at)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(cabal_id)::uuid, sqlc.arg(user_id)::uuid,
  sqlc.arg(share_units)::text::numeric, sqlc.arg(payout_micros)::text::numeric,
  sqlc.arg(sell_usdc_micros)::text::numeric, 'started', sqlc.arg(at)::timestamptz, sqlc.arg(at)::timestamptz);

-- name: CashOutJobByID :one
SELECT id, cabal_id, user_id, share_units::text AS share_units, payout_micros::text AS payout_micros,
  sell_usdc_micros::text AS sell_usdc_micros, status, result_code, created_at, updated_at
FROM cash_out_jobs
WHERE id = sqlc.arg(id)::uuid AND cabal_id = sqlc.arg(cabal_id)::uuid AND user_id = sqlc.arg(user_id)::uuid;
