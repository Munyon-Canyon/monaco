-- name: AssetByID :one
SELECT * FROM assets WHERE id = $1;

-- name: AssetByMint :one
SELECT * FROM assets WHERE mint = $1;

-- name: AssetBySymbol :one
SELECT * FROM assets WHERE symbol = $1;

-- name: ListTradableAssets :many
SELECT * FROM assets
WHERE coalesce(tradable_override, issuer_tradable)
ORDER BY popular_rank NULLS LAST, symbol;

-- name: ListAssets :many
SELECT * FROM assets ORDER BY symbol;
