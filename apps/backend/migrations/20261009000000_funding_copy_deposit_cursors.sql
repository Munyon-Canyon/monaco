INSERT INTO deposit_watch_wallets (wallet_address, user_id, first_seen_slot, first_seen_at, discovery_due_at)
SELECT c.wallet_address, w.user_id, c.cursor_slot, now(), now()
FROM deposit_cursors c
JOIN user_wallets w ON w.address = c.wallet_address
ON CONFLICT (wallet_address) DO NOTHING;
