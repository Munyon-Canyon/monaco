-- name: HeldMints :many
SELECT DISTINCT asset FROM cabal_positions WHERE units > 0 ORDER BY asset;

-- name: HeldByCabal :many
SELECT DISTINCT asset, cabal_id FROM cabal_positions WHERE units > 0;

-- name: CabalsHoldingAsset :many
SELECT DISTINCT cabal_id FROM cabal_positions WHERE units > 0 AND asset = sqlc.arg(asset)::text;
