-- name: CabalInTransitMicros :one
SELECT coalesce(sum(amount_micros), 0)::text AS micros
FROM treasury_in_transit
WHERE cabal_id = sqlc.arg(cabal_id)::uuid;

-- name: CabalValueRows :many
WITH head AS (
  SELECT
    (SELECT coalesce(sum(payout_micros), 0)
     FROM cash_out_jobs
     WHERE cabal_id = sqlc.arg(cabal_id)::uuid
       AND status IN ('started', 'selling', 'paying'))::text AS cash_out_reserved_micros,
    (SELECT coalesce(sum(amount_micros), 0)
     FROM treasury_in_transit
     WHERE cabal_id = sqlc.arg(cabal_id)::uuid)::text AS in_transit_micros
)
SELECT head.cash_out_reserved_micros, head.in_transit_micros, p.asset,
  coalesce(p.units, 0)::text AS units, coalesce(p.cost_basis_micros, 0)::text AS cost_basis_micros
FROM head
LEFT JOIN cabal_positions AS p ON p.cabal_id = sqlc.arg(cabal_id)::uuid AND p.units > 0
ORDER BY p.asset;
