-- name: UpsertEVMDepositForward :one
INSERT INTO evm_deposit_forwards AS f (
  id, user_id, network, source_tx, privy_execution_id, privy_action_id, amount_micros, status, created_at
) VALUES (
  $1, $2, $3, $4, $5, NULLIF(sqlc.arg(privy_action_id)::text, ''), sqlc.arg(amount_micros)::text::numeric, 'pending',
  $6
)
ON CONFLICT (privy_execution_id) DO UPDATE SET privy_execution_id = f.privy_execution_id
RETURNING f.id, f.user_id, f.status, f.created_at, (f.xmax = 0)::boolean AS inserted;

-- name: MarkEVMDepositForwardArrived :execrows
UPDATE evm_deposit_forwards
SET status = 'arrived', solana_signature = sqlc.arg(solana_signature), arrived_at = sqlc.arg(arrived_at)::timestamptz
WHERE id = sqlc.arg(id) AND status = 'pending';

-- name: SumPendingEVMDepositForwards :one
SELECT COALESCE(sum(amount_micros), 0)::text AS arriving_micros
FROM evm_deposit_forwards
WHERE user_id = $1 AND status = 'pending';

-- name: EVMDepositForwardStatusByExecution :one
SELECT status FROM evm_deposit_forwards WHERE privy_execution_id = $1;

-- name: RecordEVMDepositForwardProgress :execrows
UPDATE evm_deposit_forwards f
SET privy_action_id = COALESCE(f.privy_action_id, NULLIF(sqlc.arg(privy_action_id)::text, '')),
  amount_micros = sqlc.arg(amount_micros)::text::numeric,
  fee_micros = COALESCE(NULLIF(sqlc.arg(fee_micros)::text, '')::numeric, f.fee_micros),
  solana_signature = COALESCE(f.solana_signature, (
    SELECT NULLIF(sqlc.arg(solana_signature)::text, '')
    WHERE NOT EXISTS (
      SELECT 1 FROM evm_deposit_forwards o WHERE o.solana_signature = sqlc.arg(solana_signature)::text
    )
  ))
WHERE f.id = sqlc.arg(id) AND f.status = 'pending';

-- name: ArriveEVMDepositForwardFromCredit :execrows
WITH credit AS (
  SELECT d.tx_signature, d.credited_at
  FROM evm_deposit_forwards f
  JOIN deposits d ON d.user_id = f.user_id AND d.amount_micros = f.amount_micros
  WHERE f.id = sqlc.arg(id) AND f.status = 'pending' AND f.solana_signature IS NULL
    AND d.credited_at >= sqlc.arg(credited_after)::timestamptz
    AND d.credited_at <= f.created_at + make_interval(secs => sqlc.arg(window_seconds)::float8)
    AND NOT EXISTS (SELECT 1 FROM evm_deposit_forwards o WHERE o.solana_signature = d.tx_signature)
  ORDER BY abs(extract(epoch FROM d.credited_at - f.created_at)), d.id
  LIMIT 1
)
UPDATE evm_deposit_forwards f
SET status = 'arrived', solana_signature = credit.tx_signature, arrived_at = credit.credited_at
FROM credit
WHERE f.id = sqlc.arg(id) AND f.status = 'pending' AND f.solana_signature IS NULL;

-- name: MarkEVMDepositForwardFailed :execrows
UPDATE evm_deposit_forwards
SET status = 'failed'
WHERE id = $1 AND status = 'pending';

-- name: EVMDepositForwardAnnounced :one
SELECT EXISTS (
  SELECT 1 FROM events
  WHERE aggregate_type = 'evm_deposit_forward' AND aggregate_id = $1 AND type = 'deposit.evm_forwarded'
) AS announced;

-- name: ListStuckEVMDepositForwards :many
SELECT id, user_id, network, privy_execution_id, created_at
FROM evm_deposit_forwards
WHERE status = 'pending' AND created_at < sqlc.arg(created_before)
ORDER BY created_at, id
LIMIT sqlc.arg(max_rows);

-- name: ArriveEVMDepositForward :many
UPDATE evm_deposit_forwards
SET status = 'arrived', solana_signature = sqlc.arg(solana_signature)::text, arrived_at = sqlc.arg(arrived_at)::timestamptz
WHERE status = 'pending' AND id = (
  SELECT f.id
  FROM evm_deposit_forwards f
  WHERE f.user_id = sqlc.arg(user_id) AND f.status = 'pending'
    AND (
      f.solana_signature = sqlc.arg(solana_signature)::text
      OR f.solana_signature IS NULL AND f.amount_micros = sqlc.arg(amount_micros)::text::numeric
        AND f.created_at >= sqlc.arg(created_after) AND f.created_at <= sqlc.arg(arrived_at)::timestamptz
        AND NOT EXISTS (
          SELECT 1 FROM evm_deposit_forwards o WHERE o.solana_signature = sqlc.arg(solana_signature)::text
        )
    )
  ORDER BY f.solana_signature IS NULL, f.created_at, f.id
  LIMIT 1
  FOR UPDATE OF f
)
RETURNING id, amount_micros::text AS amount_micros;

-- name: LatestEVMDepositForwardFees :many
SELECT DISTINCT ON (network) network, fee_micros::text AS fee_micros
FROM evm_deposit_forwards
WHERE status = 'arrived' AND fee_micros IS NOT NULL
ORDER BY network, arrived_at DESC, id DESC;
