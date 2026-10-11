-- name: InsertTradeRoute :exec
INSERT INTO trade_routes (trade_id, comparison, winner_listing_id, created_at, pinned_network)
VALUES (
  sqlc.arg(trade_id), sqlc.arg(comparison), sqlc.arg(winner_listing_id), sqlc.arg(created_at),
  NULLIF(sqlc.arg(pinned_network)::text, '')
)
ON CONFLICT (trade_id) DO NOTHING;

-- name: FindTradeRoute :one
SELECT trade_id, comparison, winner_listing_id, created_at, coalesce(pinned_network, '')::text AS pinned_network
FROM trade_routes
WHERE trade_id = $1;

-- name: UpsertTradeStep :exec
INSERT INTO trade_steps (id, trade_id, step, network, privy_action_id, amount_micros, status, error, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
ON CONFLICT (trade_id, step) DO UPDATE
SET privy_action_id = COALESCE(trade_steps.privy_action_id, EXCLUDED.privy_action_id),
  status = EXCLUDED.status, error = EXCLUDED.error, updated_at = EXCLUDED.updated_at;

-- name: ListTradeSteps :many
SELECT id, trade_id, step, network, privy_action_id, amount_micros::text AS amount_micros, status, error,
  created_at, updated_at
FROM trade_steps
WHERE trade_id = $1
ORDER BY created_at, step;
