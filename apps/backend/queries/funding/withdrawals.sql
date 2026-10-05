-- name: LockWalletOutflow :exec
SELECT pg_advisory_xact_lock(hashtextextended('wallet-outflow:' || sqlc.arg(user_id)::uuid::text, 0));

-- name: InsertWithdrawal :exec
INSERT INTO withdrawals (id, user_id, amount_micros, to_address, status, created_at)
VALUES ($1, $2, sqlc.arg(amount_micros)::text::numeric, $3, 'created', $4);

-- name: SubmitWithdrawal :execrows
UPDATE withdrawals
SET status = 'submitted', signed_tx = sqlc.arg(signed_tx)::bytea, tx_signature = sqlc.arg(tx_signature)::text,
  last_valid_block_height = sqlc.arg(last_valid_block_height)::text::bigint,
  submitted_at = sqlc.arg(submitted_at)::timestamptz
WHERE id = $1 AND status = 'created';

-- name: FailWithdrawal :execrows
UPDATE withdrawals
SET status = 'failed', fail_code = sqlc.arg(fail_code)::text, completed_at = sqlc.arg(completed_at)::timestamptz
WHERE id = $1 AND status IN ('created', 'submitted');

-- name: GetWithdrawal :one
SELECT id, user_id, amount_micros::text AS amount_micros, to_address, status,
  COALESCE(tx_signature, '')::text AS tx_signature, COALESCE(fail_code, '')::text AS fail_code,
  created_at, completed_at
FROM withdrawals
WHERE id = $1 AND user_id = $2;

-- name: InFlightWithdrawalMicros :one
SELECT COALESCE(SUM(amount_micros), 0)::text AS micros
FROM withdrawals
WHERE user_id = $1 AND status IN ('created', 'submitted');

-- name: ListStaleCreatedWithdrawals :many
SELECT id, user_id, amount_micros::text AS amount_micros
FROM withdrawals
WHERE status = 'created' AND created_at < sqlc.arg(older_than)::timestamptz
ORDER BY created_at, id
LIMIT sqlc.arg(max_rows);

-- name: ListSubmittedWithdrawals :many
SELECT id, user_id, amount_micros::text AS amount_micros, to_address, signed_tx, tx_signature,
  last_valid_block_height
FROM withdrawals
WHERE status = 'submitted'
ORDER BY submitted_at, id
LIMIT sqlc.arg(max_rows);

-- name: ConfirmWithdrawal :execrows
UPDATE withdrawals
SET status = 'confirmed', completed_at = sqlc.arg(completed_at)::timestamptz
WHERE id = $1 AND status = 'submitted';

-- name: ListOpenWithdrawals :many
SELECT id, status, (-amount_micros)::bigint AS amount, tx_signature, created_at
FROM withdrawals
WHERE user_id = @user_id AND status <> 'confirmed'
  AND (NOT @has_cursor::bool OR (created_at, id) < (@cursor_at::timestamptz, @cursor_id::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @row_limit;
