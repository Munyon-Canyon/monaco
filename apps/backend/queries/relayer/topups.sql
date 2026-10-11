-- name: LockTopUp :exec
SELECT pg_advisory_xact_lock(hashtextextended('sol-topup:' || sqlc.arg(wallet)::text, 0));

-- name: LatestPendingTopUp :one
SELECT id, signature, last_valid_block_height::text AS last_valid_block_height
FROM relayer_sol_topups
WHERE wallet = sqlc.arg(wallet)::text AND status = 'pending'
ORDER BY created_at DESC
LIMIT 1;

-- name: ResolveTopUp :execrows
UPDATE relayer_sol_topups
SET status = sqlc.arg(status)::text, resolved_at = sqlc.arg(at)::timestamptz
WHERE id = sqlc.arg(id)::uuid AND status = 'pending';

-- name: CountTopUpsSince :one
SELECT count(*)::int AS topups
FROM relayer_sol_topups
WHERE wallet = sqlc.arg(wallet)::text AND status <> 'expired' AND created_at > sqlc.arg(since)::timestamptz;

-- name: InsertTopUp :exec
INSERT INTO relayer_sol_topups (id, wallet, lamports, signature, last_valid_block_height, status, created_at)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(wallet)::text, (sqlc.arg(lamports)::text)::numeric, sqlc.arg(signature)::text,
  (sqlc.arg(last_valid_block_height)::text)::numeric, 'pending', sqlc.arg(created_at)::timestamptz);
