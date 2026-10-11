-- name: LinkPositionListing :execrows
UPDATE cabal_positions AS p
SET listing_id = l.id
FROM asset_listings AS l
WHERE p.cabal_id = sqlc.arg(cabal_id)::uuid AND p.asset = sqlc.arg(asset)::text AND p.listing_id IS NULL
  AND CASE WHEN l.network = 'solana' THEN l.token_address ELSE l.network || ':' || l.token_address END
    = sqlc.arg(asset)::text;
