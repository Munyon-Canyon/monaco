-- name: UpsertAssets :execrows
INSERT INTO assets (
  id, symbol, mint, decimals, issuer, kind, display_name, logo_url,
  issuer_tradable, popular_rank, company_key, first_seen_at, updated_at
)
SELECT
  u.id, u.symbol, u.mint, u.decimals, sqlc.arg(issuer)::text, u.kind, u.display_name, NULLIF(u.logo_url, ''),
  u.issuer_tradable, NULLIF(u.popular_rank, 0), u.company_key, sqlc.arg(now)::timestamptz, sqlc.arg(now)::timestamptz
FROM ROWS FROM (
  unnest(sqlc.arg(ids)::uuid[]), unnest(sqlc.arg(symbols)::text[]), unnest(sqlc.arg(mints)::text[]),
  unnest(sqlc.arg(decimals)::smallint[]), unnest(sqlc.arg(kinds)::text[]), unnest(sqlc.arg(display_names)::text[]),
  unnest(sqlc.arg(logo_urls)::text[]), unnest(sqlc.arg(issuer_tradables)::bool[]),
  unnest(sqlc.arg(popular_ranks)::smallint[]), unnest(sqlc.arg(company_keys)::text[])
) AS u (id, symbol, mint, decimals, kind, display_name, logo_url, issuer_tradable, popular_rank, company_key)
ON CONFLICT (mint) DO UPDATE SET
  symbol = excluded.symbol,
  decimals = excluded.decimals,
  kind = excluded.kind,
  display_name = excluded.display_name,
  logo_url = excluded.logo_url,
  issuer_tradable = excluded.issuer_tradable,
  popular_rank = excluded.popular_rank,
  company_key = excluded.company_key,
  updated_at = excluded.updated_at
WHERE assets.issuer = excluded.issuer
  AND (
    assets.symbol, assets.decimals, assets.kind, assets.display_name, assets.logo_url,
    assets.issuer_tradable, assets.popular_rank, assets.company_key
  ) IS DISTINCT FROM (
    excluded.symbol, excluded.decimals, excluded.kind, excluded.display_name, excluded.logo_url,
    excluded.issuer_tradable, excluded.popular_rank, excluded.company_key
  );

-- name: DelistMissingAssets :execrows
UPDATE assets SET issuer_tradable = false, updated_at = sqlc.arg(now)::timestamptz
WHERE issuer = sqlc.arg(issuer)::text
  AND issuer_tradable
  AND NOT (mint = ANY (sqlc.arg(listed)::text[]));
