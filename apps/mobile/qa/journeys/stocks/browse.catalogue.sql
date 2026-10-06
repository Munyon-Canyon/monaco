WITH row_ids AS (SELECT md5(random()::text || clock_timestamp()::text || n::text) AS h, n FROM generate_series(1, 4) AS n)
INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name, issuer_tradable, tradable_override,
                    popular_rank, company_key, first_seen_at, updated_at, chain_checked_at)
VALUES
  ((SELECT (substr(r.h,1,8)||'-'||substr(r.h,9,4)||'-7'||substr(r.h,14,3)||'-8'||substr(r.h,18,3)||'-'||substr(r.h,21,12))::uuid FROM row_ids r WHERE r.n = 1), 'JRNYAx', 'HUf7vKc4ePm6sLvyNkXjCDJXpWoFRt4itqSfzL9FoAWa', 8, 'xstocks', 'equity', 'Journey Alpha', true, true, 1, 'qa-journey-alpha', now(), now(), now()),
  ((SELECT (substr(r.h,1,8)||'-'||substr(r.h,9,4)||'-7'||substr(r.h,14,3)||'-8'||substr(r.h,18,3)||'-'||substr(r.h,21,12))::uuid FROM row_ids r WHERE r.n = 2), 'JRNYPx', 'HLh7Q2NFngA22eb6ECE1d59jToPFitKBgsvYtAHuHkpn', 8, 'prestocks', 'pre_ipo', 'Journey Private', true, true, NULL, 'qa-journey-private', now(), now(), now()),
  ((SELECT (substr(r.h,1,8)||'-'||substr(r.h,9,4)||'-7'||substr(r.h,14,3)||'-8'||substr(r.h,18,3)||'-'||substr(r.h,21,12))::uuid FROM row_ids r WHERE r.n = 3), 'JRNYQx', 'FHog94s1JLjAgq7UDDrvckRZhe6waFpeFtWCTFimdgNL', 8, 'tessera', 'pre_ipo', 'Journey Private', true, true, NULL, 'qa-journey-private', now(), now(), now()),
  ((SELECT (substr(r.h,1,8)||'-'||substr(r.h,9,4)||'-7'||substr(r.h,14,3)||'-8'||substr(r.h,18,3)||'-'||substr(r.h,21,12))::uuid FROM row_ids r WHERE r.n = 4), 'JRNYZx', '5dDnwzhRKD48h1vfrq4akL2hcKksfab4XCnEX7gRyrE4', 8, 'xstocks', 'equity', 'Journey Zulu', true, true, NULL, 'qa-journey-zulu', now(), now(), now())
ON CONFLICT (symbol) DO UPDATE SET
  id = excluded.id,  mint = excluded.mint, decimals = excluded.decimals, issuer = excluded.issuer, kind = excluded.kind,
  display_name = excluded.display_name, logo_url = NULL, issuer_tradable = true, tradable_override = true,
  popular_rank = excluded.popular_rank, company_key = excluded.company_key, updated_at = now(), chain_checked_at = now();

DELETE FROM price_points WHERE source = 'qa-journey';

INSERT INTO price_points (mint, ts, price_micros, source)
SELECT a.mint, s.ts, p.price_micros, 'qa-journey'
FROM (VALUES
  ('JRNYAx', 123450000::bigint), ('JRNYPx', 50000000::bigint), ('JRNYQx', 51000000::bigint), ('JRNYZx', 10000000::bigint)
) AS p(symbol, price_micros)
JOIN assets a ON a.symbol = p.symbol
CROSS JOIN (VALUES (now() - interval '12 hours'), (now())) AS s(ts);

SELECT count(*) FROM price_points WHERE source = 'qa-journey';
