-- name: InsertDeposit :execrows
INSERT INTO deposits (
  id, user_id, wallet_address, tx_signature, amount_micros, slot, block_time, credited_at
) VALUES ($1, $2, $3, $4, sqlc.arg(amount_micros)::text::numeric, $5,
  NULLIF(sqlc.arg(block_time)::timestamptz, '0001-01-01 00:00:00+00'::timestamptz), $6)
ON CONFLICT (tx_signature, wallet_address) DO NOTHING;

-- name: InsertDepositCandidate :execrows
INSERT INTO deposit_candidates (
  tx_signature, wallet_address, user_id, slot, block_time, source, status, seen_at
) VALUES ($1, $2, $3, $4,
  NULLIF(sqlc.arg(block_time)::timestamptz, '0001-01-01 00:00:00+00'::timestamptz),
  $5, 'pending', $6)
ON CONFLICT (tx_signature, wallet_address) DO NOTHING;

-- name: ResolveDepositCandidate :execrows
UPDATE deposit_candidates
SET status = $1,
    resolved_at = NULLIF(sqlc.arg(resolved_at)::timestamptz, '0001-01-01T00:00:00Z'::timestamptz)
WHERE tx_signature = $2 AND wallet_address = $3 AND status = 'pending';

-- name: InsertDepositWatchWallet :execrows
INSERT INTO deposit_watch_wallets (
  wallet_address, user_id, first_seen_slot, first_seen_at, discovery_due_at, opening_micros, reconcile_due_at
) VALUES ($1, $2, $3, $4, sqlc.arg(discovery_due_at)::timestamptz,
  NULLIF(sqlc.arg(opening_micros)::text, '')::numeric, sqlc.arg(reconcile_due_at)::timestamptz)
ON CONFLICT (wallet_address) DO NOTHING;

-- name: InsertDepositWatchAccount :exec
INSERT INTO deposit_watch_accounts (
  token_account, wallet_address, canonical, state, last_amount, observed_slot,
  dirty_gen, dirty_slot, high_signature, high_slot, recovery_due_at
) VALUES ($1, $2, $3, $4, sqlc.arg(last_amount)::text::numeric, sqlc.arg(observed_slot)::bigint,
  sqlc.arg(dirty_gen)::bigint, sqlc.arg(dirty_slot)::bigint,
  NULLIF(sqlc.arg(high_signature)::text, ''), sqlc.arg(high_slot)::bigint, sqlc.arg(recovery_due_at)::timestamptz)
ON CONFLICT (token_account) DO NOTHING;

-- name: DepositWatchDirtyAccounts :many
SELECT a.token_account, a.wallet_address, w.user_id, w.first_seen_slot, a.dirty_gen, a.dirty_slot, a.observed_slot,
  a.high_signature, a.page_before
FROM deposit_watch_accounts a
JOIN deposit_watch_wallets w ON w.wallet_address = a.wallet_address
WHERE a.dirty_gen > a.clean_gen AND a.state <> 'foreign'
ORDER BY a.dirty_slot, a.token_account
LIMIT $1;

-- name: DepositWatchKnownWallets :many
SELECT wallet_address
FROM deposit_watch_wallets
WHERE wallet_address = ANY(sqlc.arg(wallet_addresses)::text[]);

-- name: MarkDepositWatchAccountDirty :execrows
UPDATE deposit_watch_accounts
SET dirty_gen = dirty_gen + 1, dirty_slot = GREATEST(dirty_slot, sqlc.arg(slot)::bigint)
WHERE token_account = $1;

-- name: CheckpointDepositWatchPage :exec
UPDATE deposit_watch_accounts
SET page_before = sqlc.arg(page_before)::text,
    page_top_signature = COALESCE(page_top_signature, sqlc.arg(page_top_signature)::text),
    page_top_slot = COALESCE(page_top_slot, sqlc.arg(page_top_slot)::bigint),
    scanned_at = sqlc.arg(scanned_at)::timestamptz
WHERE token_account = $1;

-- name: CompleteDepositWatchPage :exec
UPDATE deposit_watch_accounts
SET high_signature = COALESCE(page_top_signature, high_signature),
    high_slot = COALESCE(page_top_slot, high_slot),
    page_before = NULL,
    page_top_signature = NULL,
    page_top_slot = NULL,
    clean_gen = $2,
    scanned_at = sqlc.arg(scanned_at)::timestamptz
WHERE token_account = $1 AND clean_gen < $2;

-- name: DepositWatchGateAccounts :many
SELECT token_account, wallet_address, state, last_amount::text AS last_amount, observed_slot
FROM deposit_watch_accounts
WHERE state <> 'foreign' AND token_account > $1
ORDER BY token_account
LIMIT $2;

-- name: ApplyDepositWatchObservation :execrows
UPDATE deposit_watch_accounts
SET state = sqlc.arg(state)::text,
    last_amount = sqlc.arg(last_amount)::text::numeric,
    observed_slot = sqlc.arg(observed_slot)::bigint,
    dirty_gen = dirty_gen + CASE WHEN sqlc.arg(dirty)::bool THEN 1 ELSE 0 END,
    dirty_slot = CASE WHEN sqlc.arg(dirty)::bool THEN GREATEST(dirty_slot, sqlc.arg(observed_slot)::bigint)
      ELSE dirty_slot END
WHERE token_account = $1 AND observed_slot <= sqlc.arg(observed_slot)::bigint AND state <> 'foreign';

-- name: DepositWatchRotationAccounts :many
SELECT a.token_account, a.wallet_address, w.user_id, w.first_seen_slot, a.high_slot, a.recovery_before
FROM deposit_watch_accounts a
JOIN deposit_watch_wallets w ON w.wallet_address = a.wallet_address
WHERE a.recovery_due_at <= sqlc.arg(now)::timestamptz AND a.state <> 'foreign'
ORDER BY a.recovery_due_at, a.token_account
LIMIT sqlc.arg(row_limit)::int;

-- name: CheckpointDepositWatchRecovery :exec
UPDATE deposit_watch_accounts
SET recovery_before = sqlc.arg(recovery_before)::text,
    scanned_at = sqlc.arg(scanned_at)::timestamptz
WHERE token_account = $1;

-- name: CompleteDepositWatchRecovery :exec
UPDATE deposit_watch_accounts
SET recovery_before = NULL,
    recovery_due_at = sqlc.arg(recovery_due_at)::timestamptz,
    scanned_at = sqlc.arg(scanned_at)::timestamptz
WHERE token_account = $1;

-- name: DepositWatchDiscoveryWallets :many
SELECT wallet_address, user_id
FROM deposit_watch_wallets
WHERE discovery_due_at IS NULL OR discovery_due_at <= sqlc.arg(now)::timestamptz
ORDER BY discovery_due_at NULLS FIRST, wallet_address
LIMIT sqlc.arg(row_limit)::int;

-- name: SetDepositWatchDiscovery :exec
UPDATE deposit_watch_wallets
SET discovery_due_at = sqlc.arg(discovery_due_at)::timestamptz
WHERE wallet_address = $1;

-- name: DepositWatchReconcileWallets :many
SELECT w.wallet_address, w.user_id, coalesce(w.opening_micros::text, '')::text AS opening_micros, w.residual_streak,
  coalesce(sum(a.last_amount) FILTER (WHERE a.state = 'open'), 0)::text AS observed,
  coalesce(bool_or(a.dirty_gen > a.clean_gen), false)::boolean AS dirty,
  EXISTS (
    SELECT 1 FROM deposit_candidates c WHERE c.wallet_address = w.wallet_address AND c.status = 'pending'
  )::boolean AS pending_candidates
FROM deposit_watch_wallets w
JOIN deposit_watch_accounts a ON a.wallet_address = w.wallet_address
WHERE w.reconcile_due_at <= sqlc.arg(now)::timestamptz
GROUP BY w.wallet_address
HAVING bool_and(a.observed_slot > 0)
ORDER BY w.reconcile_due_at, w.wallet_address
LIMIT sqlc.arg(row_limit)::int;

-- name: SetDepositWatchReconcile :exec
UPDATE deposit_watch_wallets
SET opening_micros = NULLIF(sqlc.arg(opening_micros)::text, '')::numeric,
    residual_streak = sqlc.arg(residual_streak)::int,
    reconcile_due_at = sqlc.arg(reconcile_due_at)::timestamptz
WHERE wallet_address = $1;

-- name: DepositWatchResidualWallets :one
SELECT count(*)::bigint FROM deposit_watch_wallets WHERE residual_streak >= 2;

-- name: DepositCandidatesPendingOldestSeconds :one
SELECT coalesce(extract(epoch FROM (sqlc.arg(now)::timestamptz - min(seen_at))), 0)::bigint
FROM deposit_candidates WHERE status = 'pending';
