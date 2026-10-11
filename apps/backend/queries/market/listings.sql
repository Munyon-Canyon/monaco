-- name: ListEnabledListingsForAsset :many
SELECT id, asset_id, issuer, symbol, network, chain_id, token_address, wrapper_address, decimals,
  shares_per_token::text AS shares_per_token, venue, enabled, verified_at, created_at
FROM asset_listings
WHERE asset_id = $1 AND enabled
ORDER BY network, issuer;

-- name: FindListing :one
SELECT id, asset_id, issuer, symbol, network, chain_id, token_address, wrapper_address, decimals,
  shares_per_token::text AS shares_per_token, venue, enabled, verified_at, created_at
FROM asset_listings
WHERE id = $1;

-- name: FindListingByToken :one
SELECT id, asset_id, issuer, symbol, network, chain_id, token_address, wrapper_address, decimals,
  shares_per_token::text AS shares_per_token, venue, enabled, verified_at, created_at
FROM asset_listings
WHERE network = $1 AND token_address = $2;
