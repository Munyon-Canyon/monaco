-- name: UpsertEVMDepositForward :one
INSERT INTO evm_deposit_forwards AS f (
  id, user_id, network, source_tx, privy_execution_id, privy_action_id, amount_micros, status, created_at
) VALUES (
  $1, $2, $3, $4, $5, NULLIF(sqlc.arg(privy_action_id)::text, ''), sqlc.arg(amount_micros)::text::numeric, 'pending',
  $6
)
ON CONFLICT (privy_execution_id) DO UPDATE SET privy_execution_id = f.privy_execution_id
RETURNING f.id, f.user_id, f.status, (f.xmax = 0)::boolean AS inserted;

-- name: MarkEVMDepositForwardArrived :execrows
UPDATE evm_deposit_forwards
SET status = 'arrived', solana_signature = sqlc.arg(solana_signature), arrived_at = sqlc.arg(arrived_at)
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
  solana_signature = COALESCE(f.solana_signature, (
    SELECT NULLIF(sqlc.arg(solana_signature)::text, '')
    WHERE NOT EXISTS (
      SELECT 1 FROM evm_deposit_forwards o WHERE o.solana_signature = sqlc.arg(solana_signature)::text
    )
  ))
WHERE f.id = sqlc.arg(id) AND f.status = 'pending';

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
