-- name: LockWalletOutflow :exec
SELECT pg_advisory_xact_lock(hashtextextended('wallet-outflow:' || sqlc.arg(user_id)::uuid::text, 0));
