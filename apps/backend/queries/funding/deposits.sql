-- name: InsertDeposit :execrows
INSERT INTO deposits (
  id, user_id, wallet_address, tx_signature, amount_micros, slot, block_time, credited_at
) VALUES ($1, $2, $3, $4, sqlc.arg(amount_micros)::text::numeric, $5,
  NULLIF(sqlc.arg(block_time)::timestamptz, '0001-01-01 00:00:00+00'::timestamptz), $6)
ON CONFLICT (tx_signature, wallet_address) DO NOTHING;

-- name: AdvanceDepositCursor :exec
INSERT INTO deposit_cursors (wallet_address, last_signature, cursor_slot, scanned_at)
VALUES ($1, sqlc.arg(last_signature)::text, $2, $3)
ON CONFLICT (wallet_address) DO UPDATE
SET last_signature = EXCLUDED.last_signature,
    cursor_slot = EXCLUDED.cursor_slot,
    scanned_at = EXCLUDED.scanned_at
WHERE EXCLUDED.cursor_slot >= deposit_cursors.cursor_slot;

-- name: DepositCursorsForWallets :many
WITH wallets AS (SELECT unnest(sqlc.arg(wallet_addresses)::text[])::text AS wallet_address)
SELECT wallets.wallet_address,
  COALESCE(deposit_cursors.last_signature, '') AS last_signature,
  COALESCE(deposit_cursors.cursor_slot, 0) AS cursor_slot,
  COALESCE(deposit_cursors.backfill_before_signature, '') AS backfill_before_signature,
  COALESCE(deposit_cursors.backfill_head_signature, '') AS backfill_head_signature,
  COALESCE(deposit_cursors.backfill_head_slot, 0) AS backfill_head_slot,
  COALESCE(deposit_cursors.scanned_at, to_timestamp(0)) AS scanned_at,
  (deposit_cursors.wallet_address IS NOT NULL)::bool AS exists
FROM wallets
LEFT JOIN deposit_cursors ON deposit_cursors.wallet_address = wallets.wallet_address
ORDER BY COALESCE(deposit_cursors.scanned_at, to_timestamp(0));

-- name: SetDepositBackfill :exec
UPDATE deposit_cursors
SET backfill_before_signature = sqlc.arg(before_signature)::text,
    backfill_head_signature = sqlc.arg(head_signature)::text,
    backfill_head_slot = sqlc.arg(head_slot)::bigint
WHERE wallet_address = $1;

-- name: TouchDepositCursor :exec
INSERT INTO deposit_cursors (wallet_address, last_signature, cursor_slot, scanned_at)
VALUES ($1, '', 0, $2)
ON CONFLICT (wallet_address) DO UPDATE SET scanned_at = EXCLUDED.scanned_at;

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

-- name: DepositCursorsToMigrate :many
SELECT c.wallet_address, w.user_id, COALESCE(c.last_signature, '') AS last_signature, c.cursor_slot
FROM deposit_cursors c
JOIN user_wallets w ON w.address = c.wallet_address
WHERE c.wallet_address > $1
ORDER BY c.wallet_address
LIMIT $2;

-- name: InsertDepositWatchWallet :execrows
INSERT INTO deposit_watch_wallets (wallet_address, user_id, first_seen_slot, first_seen_at)
VALUES ($1, $2, $3, $4)
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
SELECT a.token_account, a.wallet_address, w.user_id, a.dirty_gen, a.dirty_slot, a.observed_slot,
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
