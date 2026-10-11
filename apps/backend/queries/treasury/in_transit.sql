-- name: InsertInTransit :exec
INSERT INTO treasury_in_transit (cabal_id, trade_id, network, amount_micros, created_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (trade_id, network) DO NOTHING;

-- name: DeleteInTransit :execrows
DELETE FROM treasury_in_transit WHERE trade_id = $1 AND network = $2;

-- name: SumInTransit :one
SELECT COALESCE(sum(amount_micros), 0)::text AS in_transit_micros
FROM treasury_in_transit
WHERE cabal_id = $1;

-- name: SetPositionListing :execrows
UPDATE cabal_positions SET listing_id = $3
WHERE cabal_id = $1 AND asset = $2 AND listing_id IS NULL;
