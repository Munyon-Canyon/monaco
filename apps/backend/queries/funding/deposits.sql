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

-- name: DepositCursor :one
SELECT COALESCE(deposit_cursors.last_signature, '') AS last_signature, deposit_cursors.scanned_at
FROM deposit_cursors
WHERE deposit_cursors.wallet_address = $1
UNION ALL
SELECT '', to_timestamp(0)
WHERE NOT EXISTS (SELECT 1 FROM deposit_cursors WHERE wallet_address = $1)
LIMIT 1;

-- name: TouchDepositCursor :exec
INSERT INTO deposit_cursors (wallet_address, last_signature, cursor_slot, scanned_at)
VALUES ($1, '', 0, $2)
ON CONFLICT (wallet_address) DO UPDATE SET scanned_at = EXCLUDED.scanned_at;
