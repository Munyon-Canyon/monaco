-- name: CopyTradeRoute :exec
INSERT INTO trade_routes (trade_id, comparison, winner_listing_id, created_at)
SELECT sqlc.arg(to_trade)::uuid, r.comparison, r.winner_listing_id, sqlc.arg(at)::timestamptz
FROM trade_routes AS r
WHERE r.trade_id = sqlc.arg(from_trade)::uuid
ON CONFLICT (trade_id) DO NOTHING;
