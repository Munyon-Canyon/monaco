-- name: LockWalletOutflow :exec
SELECT pg_advisory_xact_lock(hashtextextended('wallet-outflow:' || sqlc.arg(user_id)::uuid::text, 0));

-- name: InsertFundTransfer :exec
INSERT INTO fund_transfers (id, user_id, cabal_id, amount_micros, from_address, to_address, status, created_at)
VALUES ($1, $2, $3, sqlc.arg(amount_micros)::text::numeric, $4, $5, 'created', $6);

-- name: SubmitFundTransfer :execrows
UPDATE fund_transfers
SET status = 'submitted', signed_tx = sqlc.arg(signed_tx)::bytea, tx_signature = sqlc.arg(tx_signature)::text,
  last_valid_block_height = sqlc.arg(last_valid_block_height)::text::bigint,
  submitted_at = sqlc.arg(submitted_at)::timestamptz
WHERE id = $1 AND status = 'created';

-- name: LandFundTransfer :execrows
UPDATE fund_transfers SET status = 'landed', landed_at = sqlc.arg(landed_at)::timestamptz
WHERE id = $1 AND status = 'submitted';

-- name: SettleFundTransfer :execrows
UPDATE fund_transfers
SET status = 'settled', share_units = sqlc.arg(share_units)::text::numeric,
  settled_at = sqlc.arg(settled_at)::timestamptz
WHERE id = $1 AND status = 'landed';

-- name: FailFundTransfer :execrows
UPDATE fund_transfers SET status = 'failed', fail_code = sqlc.arg(fail_code)::text
WHERE id = $1 AND status IN ('created', 'submitted');

-- name: GetFundTransfer :one
SELECT id, user_id, cabal_id, amount_micros::text AS amount_micros, status,
  COALESCE(share_units, 0)::text AS share_units, COALESCE(fail_code, '')::text AS fail_code
FROM fund_transfers
WHERE id = $1;

-- name: InFlightFundMicros :one
SELECT COALESCE(SUM(amount_micros), 0)::text AS micros
FROM fund_transfers
WHERE user_id = $1 AND status IN ('created', 'submitted');

-- name: ExpireCreatedFundTransfers :many
UPDATE fund_transfers SET status = 'failed', fail_code = 'fund_not_sent'
WHERE status = 'created' AND created_at < sqlc.arg(cutoff)::timestamptz
RETURNING id;

-- name: ListOpenFundTransfers :many
SELECT id, user_id, cabal_id, amount_micros::text AS amount_micros, status,
  COALESCE(signed_tx, ''::bytea)::bytea AS signed_tx, COALESCE(tx_signature, '')::text AS tx_signature,
  COALESCE(last_valid_block_height, 0)::bigint AS last_valid_block_height
FROM fund_transfers
WHERE status IN ('submitted', 'landed')
ORDER BY created_at, id
LIMIT sqlc.arg(row_limit);
