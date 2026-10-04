INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name, issuer_tradable, tradable_override,
                    popular_rank, company_key, first_seen_at, updated_at, chain_checked_at)
VALUES
  (gen_random_uuid(), 'JRNYAx', 'QAJourneyAlphaMint1111111111111111111111111', 8, 'xstocks', 'equity', 'Journey Alpha', true, true, 1, 'qa-journey-alpha', now(), now(), now()),
  (gen_random_uuid(), 'JRNYPx', 'QAJourneyPrivateMint11111111111111111111111', 8, 'prestocks', 'pre_ipo', 'Journey Private', true, true, NULL, 'qa-journey-private', now(), now(), now()),
  (gen_random_uuid(), 'JRNYQx', 'QAJourneyPrivateTesseraMint111111111111111', 8, 'tessera', 'pre_ipo', 'Journey Private', true, true, NULL, 'qa-journey-private', now(), now(), now()),
  (gen_random_uuid(), 'JRNYZx', 'QAJourneyZuluMint11111111111111111111111111', 8, 'xstocks', 'equity', 'Journey Zulu', true, true, NULL, 'qa-journey-zulu', now(), now(), now())
ON CONFLICT (symbol) DO UPDATE SET
  mint = excluded.mint, decimals = excluded.decimals, issuer = excluded.issuer, kind = excluded.kind,
  display_name = excluded.display_name, logo_url = NULL, issuer_tradable = true, tradable_override = true,
  popular_rank = excluded.popular_rank, company_key = excluded.company_key, updated_at = now(), chain_checked_at = now();

DELETE FROM price_points WHERE mint LIKE 'QAJourney%';

INSERT INTO price_points (mint, ts, price_micros, source)
SELECT a.mint, s.ts, p.price_micros, 'qa-journey'
FROM (VALUES
  ('JRNYAx', 123450000::bigint), ('JRNYPx', 50000000::bigint), ('JRNYQx', 51000000::bigint), ('JRNYZx', 10000000::bigint)
) AS p(symbol, price_micros)
JOIN assets a ON a.symbol = p.symbol
CROSS JOIN (VALUES (now() - interval '1 day'), (now())) AS s(ts);

SELECT count(*) FROM price_points WHERE mint LIKE 'QAJourney%';
