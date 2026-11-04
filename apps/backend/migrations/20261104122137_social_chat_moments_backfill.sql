INSERT INTO cabal_messages (id, cabal_id, kind, moment, dedupe_key, created_at)
SELECT gen_random_uuid(), c.id, 'system', jsonb_build_object('kind', 'cabal_created'), 'created', c.created_at
FROM cabals c
ON CONFLICT (cabal_id, dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING;

INSERT INTO cabal_messages (id, cabal_id, kind, moment, dedupe_key, created_at)
SELECT
  gen_random_uuid(), m.cabal_id, 'system',
  jsonb_build_object('kind', 'member_joined', 'user_id', m.user_id),
  'joined:' || m.user_id::text, m.joined_at
FROM cabal_members m
WHERE NOT EXISTS (
  SELECT 1 FROM cabal_messages x
  WHERE x.cabal_id = m.cabal_id AND x.kind = 'system'
    AND x.moment ->> 'kind' = 'member_joined' AND x.moment ->> 'user_id' = m.user_id::text
)
ON CONFLICT (cabal_id, dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING;

INSERT INTO cabal_messages (id, cabal_id, kind, moment, dedupe_key, created_at)
SELECT
  gen_random_uuid(), f.cabal_id, 'system',
  jsonb_build_object(
    'kind', 'trade_filled', 'side', f.action, 'symbol', f.symbol, 'asset_id', f.asset_id,
    'price_micros', f.price_micros, 'usdc_micros', f.usdc_micros,
    'filled_at', to_char(f.confirmed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
    'swap_id', f.swap_id, 'proposal_id', f.source_id
  ),
  'fill:' || f.swap_id::text, f.confirmed_at
FROM (
  SELECT
    s.id AS swap_id, s.cabal_id, s.source_id, s.action, s.confirmed_at, a.id AS asset_id, a.symbol,
    t.usdc_micros, div(t.usdc_micros * power(10::numeric, a.decimals), t.tokens) AS price_micros
  FROM swaps s
  JOIN cabals c ON c.id = s.cabal_id
  JOIN assets a ON a.mint = CASE WHEN s.action = 'sell' THEN s.in_mint ELSE s.out_mint END
  CROSS JOIN LATERAL (
    SELECT
      CASE WHEN s.action = 'sell' THEN s.out_amount ELSE s.in_amount END AS usdc_micros,
      CASE WHEN s.action = 'sell' THEN s.in_amount ELSE s.out_amount END AS tokens
  ) t
  WHERE s.status = 'confirmed' AND s.source_kind = 'proposal' AND s.confirmed_at IS NOT NULL
    AND t.usdc_micros > 0 AND t.usdc_micros <= 9223372036854775807 AND t.tokens > 0
) f
WHERE f.price_micros BETWEEN 1 AND 9223372036854775807
ON CONFLICT (cabal_id, dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING;
