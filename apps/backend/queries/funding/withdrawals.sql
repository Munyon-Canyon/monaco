-- name: LockWalletOutflow :exec
SELECT pg_advisory_xact_lock(hashtextextended('wallet-outflow:' || sqlc.arg(user_id)::uuid::text, 0));

-- name: InsertWithdrawal :exec
INSERT INTO withdrawals (id, user_id, amount_micros, to_address, status, created_at, destination_kind, network,
  evm_address)
VALUES ($1, $2, sqlc.arg(amount_micros)::text::numeric, $3, 'created', $4, sqlc.arg(destination_kind)::text,
  sqlc.arg(network)::text, NULLIF(sqlc.arg(evm_address)::text, ''));

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

-- name: UserSubmittedWithdrawals :many
SELECT COALESCE(tx_signature, '')::text AS tx_signature, amount_micros::text AS amount_micros
FROM withdrawals
WHERE user_id = $1 AND status = 'submitted' AND tx_signature IS NOT NULL;

-- name: LastWithdrawalChange :one
SELECT COALESCE(MAX(GREATEST(created_at, submitted_at, completed_at)), 'epoch')::timestamptz AS changed_at
FROM withdrawals
WHERE user_id = $1;

-- name: ListStaleCreatedWithdrawals :many
SELECT id, user_id, amount_micros::text AS amount_micros
FROM withdrawals
WHERE status = 'created' AND destination_kind = 'solana' AND created_at < sqlc.arg(older_than)::timestamptz
ORDER BY created_at, id
LIMIT sqlc.arg(max_rows);

-- name: ListSubmittedWithdrawals :many
SELECT id, user_id, amount_micros::text AS amount_micros, to_address, signed_tx, tx_signature,
  last_valid_block_height
FROM withdrawals
WHERE status = 'submitted' AND destination_kind = 'solana'
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

-- name: SubmitEVMWithdrawal :execrows
UPDATE withdrawals
SET status = 'submitted', privy_action_id = sqlc.arg(privy_action_id)::text,
  submitted_at = sqlc.arg(submitted_at)::timestamptz
WHERE id = $1 AND status = 'created' AND destination_kind = 'evm';

-- name: ListStaleCreatedEVMWithdrawals :many
SELECT id, user_id, amount_micros::text AS amount_micros, network, evm_address::text AS evm_address
FROM withdrawals
WHERE status = 'created' AND destination_kind = 'evm' AND created_at < sqlc.arg(older_than)::timestamptz
ORDER BY created_at, id
LIMIT sqlc.arg(max_rows);

-- name: ListSubmittedEVMWithdrawals :many
SELECT id, user_id, amount_micros::text AS amount_micros, evm_address::text AS evm_address,
  privy_action_id::text AS privy_action_id, COALESCE(tx_signature, '')::text AS tx_signature
FROM withdrawals
WHERE status = 'submitted' AND destination_kind = 'evm'
ORDER BY submitted_at, id
LIMIT sqlc.arg(max_rows);

-- name: SetWithdrawalSourceSignature :execrows
UPDATE withdrawals SET tx_signature = NULLIF(sqlc.arg(tx_signature)::text, '')
WHERE id = $1 AND status = 'submitted' AND destination_kind = 'evm' AND tx_signature IS NULL;

-- name: FreshWithdrawalDestinations :one
SELECT count(*)::bigint AS fresh_today,
  EXISTS (
    SELECT 1 FROM withdrawals k
    WHERE k.destination_kind = 'solana' AND k.to_address = sqlc.arg(to_address)::text
      AND k.status IN ('submitted', 'confirmed')
  ) AS destination_known,
  EXISTS (
    SELECT 1 FROM withdrawals k
    WHERE k.destination_kind = 'solana' AND k.to_address = sqlc.arg(to_address)::text AND k.status = 'confirmed'
  ) AS destination_landed
FROM withdrawals w
WHERE w.user_id = sqlc.arg(user_id)::uuid AND w.destination_kind = 'solana' AND w.status <> 'failed'
  AND w.created_at >= sqlc.arg(since)::timestamptz
  AND NOT EXISTS (
    SELECT 1 FROM withdrawals p
    WHERE p.destination_kind = 'solana' AND p.to_address = w.to_address
      AND p.status IN ('submitted', 'confirmed') AND (p.created_at, p.id) < (w.created_at, w.id)
  );
