-- name: InsertEVMDepositForward :execrows
INSERT INTO evm_deposit_forwards (
  id, user_id, network, source_tx, privy_execution_id, privy_action_id, amount_micros, status, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending', $8)
ON CONFLICT (privy_execution_id) DO NOTHING;

-- name: MarkEVMDepositForwardArrived :execrows
UPDATE evm_deposit_forwards
SET status = 'arrived', solana_signature = sqlc.arg(solana_signature), arrived_at = sqlc.arg(arrived_at)
WHERE id = sqlc.arg(id) AND status = 'pending';

-- name: SumPendingEVMDepositForwards :one
SELECT COALESCE(sum(amount_micros), 0)::text AS arriving_micros
FROM evm_deposit_forwards
WHERE user_id = $1 AND status = 'pending';
