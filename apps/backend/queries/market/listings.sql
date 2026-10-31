-- name: ListEnabledListingsForAsset :many
SELECT l.id, l.asset_id, l.issuer, l.symbol, l.network, l.chain_id, l.token_address, l.wrapper_address, l.decimals,
  l.shares_per_token::text AS shares_per_token, l.venue, l.enabled, l.verified_at, l.created_at
FROM asset_listings AS l
JOIN assets AS a ON a.id = l.asset_id
WHERE l.asset_id = $1 AND l.enabled AND (l.network <> 'solana' OR l.token_address = a.mint)
ORDER BY l.network, l.issuer;

-- name: FindListingForAssetOn :one
SELECT id, venue
FROM asset_listings
WHERE asset_id = @asset_id::uuid AND network = @network::text
ORDER BY enabled DESC, verified_at IS NULL, issuer, token_address
LIMIT 1;

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

-- name: UpsertListings :execrows
INSERT INTO asset_listings (
  id, asset_id, issuer, symbol, network, chain_id, token_address, wrapper_address, decimals,
  shares_per_token, venue, issuer_tradable, tradable_override, enabled, created_at
)
SELECT
  u.id,
  (SELECT a.id FROM assets AS a WHERE a.issuer = 'xstocks' AND a.symbol = u.ticker || 'x'),
  sqlc.arg(issuer)::text, u.symbol, u.network, NULLIF(u.chain_id, 0), u.token_address, NULLIF(u.wrapper_address, ''),
  NULLIF(u.decimals, -1),
  coalesce(
    NULLIF(u.shares_per_token, '')::numeric,
    (SELECT a.ui_multiplier_num::numeric / a.ui_multiplier_den FROM assets AS a
      WHERE a.issuer = 'xstocks' AND a.symbol = u.ticker || 'x' AND a.ui_multiplier_den > 0),
    1
  ),
  CASE WHEN u.network = 'solana' THEN 'jupiter' ELSE 'uniswap' END,
  u.issuer_tradable,
  CASE WHEN sqlc.arg(issuer)::text = 'xstocks' AND u.network NOT IN ('solana', 'arbitrum') THEN false END,
  false, sqlc.arg(now)::timestamptz
FROM ROWS FROM (
  unnest(sqlc.arg(ids)::uuid[]), unnest(sqlc.arg(tickers)::text[]), unnest(sqlc.arg(symbols)::text[]),
  unnest(sqlc.arg(networks)::text[]), unnest(sqlc.arg(chain_ids)::bigint[]), unnest(sqlc.arg(token_addresses)::text[]),
  unnest(sqlc.arg(wrapper_addresses)::text[]), unnest(sqlc.arg(decimals)::smallint[]),
  unnest(sqlc.arg(shares_per_tokens)::text[]), unnest(sqlc.arg(issuer_tradables)::bool[])
) AS u (
  id, ticker, symbol, network, chain_id, token_address, wrapper_address, decimals, shares_per_token, issuer_tradable
)
ON CONFLICT (issuer, network, token_address) DO UPDATE SET
  asset_id = coalesce(asset_listings.asset_id, excluded.asset_id),
  symbol = excluded.symbol,
  wrapper_address = excluded.wrapper_address,
  decimals = CASE WHEN asset_listings.verified_at IS NULL THEN excluded.decimals ELSE asset_listings.decimals END,
  shares_per_token = excluded.shares_per_token,
  issuer_tradable = excluded.issuer_tradable,
  enabled = asset_listings.verified_at IS NOT NULL AND coalesce(asset_listings.tradable_override, excluded.issuer_tradable)
WHERE (
  asset_listings.asset_id, asset_listings.symbol, asset_listings.wrapper_address, asset_listings.decimals,
  asset_listings.shares_per_token, asset_listings.issuer_tradable
) IS DISTINCT FROM (
  coalesce(asset_listings.asset_id, excluded.asset_id), excluded.symbol, excluded.wrapper_address,
  CASE WHEN asset_listings.verified_at IS NULL THEN excluded.decimals ELSE asset_listings.decimals END,
  excluded.shares_per_token, excluded.issuer_tradable
);

-- name: HaltMissingListings :many
UPDATE asset_listings SET issuer_tradable = false, enabled = false
WHERE issuer = sqlc.arg(issuer)::text AND symbol = ANY (sqlc.arg(symbols)::text[]) AND issuer_tradable
RETURNING symbol, network;

-- name: ListListingsToVerify :many
SELECT id, issuer, symbol, network, token_address, wrapper_address, decimals
FROM asset_listings
WHERE verified_at IS NULL OR sqlc.arg(all_listings)::bool
ORDER BY issuer, network, symbol;

-- name: MarkListingVerified :execrows
UPDATE asset_listings SET
  decimals = sqlc.arg(decimals)::smallint,
  verified_at = sqlc.arg(verified_at)::timestamptz,
  enabled = coalesce(tradable_override, issuer_tradable)
WHERE id = sqlc.arg(id);

-- name: MarkListingUnverified :execrows
UPDATE asset_listings SET verified_at = NULL, enabled = false
WHERE id = sqlc.arg(id) AND verified_at IS NOT NULL;

-- name: SetListingTradableOverride :one
UPDATE asset_listings SET
  tradable_override = CASE sqlc.arg(override)::text WHEN 'auto' THEN NULL ELSE sqlc.arg(override)::text = 'on' END,
  enabled = verified_at IS NOT NULL AND CASE sqlc.arg(override)::text
    WHEN 'auto' THEN issuer_tradable ELSE sqlc.arg(override)::text = 'on' END
WHERE id = sqlc.arg(id)
RETURNING id, issuer, symbol, network, token_address, enabled;

-- name: AssetByListingToken :one
SELECT a.id, a.symbol, l.token_address AS mint, l.decimals, a.issuer, a.kind, a.display_name, a.logo_url,
  a.ui_multiplier_num, a.ui_multiplier_den, a.issuer_tradable, a.tradable_override, a.popular_rank, a.company_key,
  a.first_seen_at, a.updated_at, a.chain_checked_at, a.ui_multiplier_next_num, a.ui_multiplier_next_den,
  a.ui_multiplier_next_at, a.last_quoted_at, l.shares_per_token::text AS shares_per_token
FROM asset_listings AS l
JOIN assets AS a ON a.id = l.asset_id
WHERE l.network = sqlc.arg(network)::text AND l.token_address = sqlc.arg(token_address)::text
  AND l.decimals IS NOT NULL AND l.verified_at IS NOT NULL;
