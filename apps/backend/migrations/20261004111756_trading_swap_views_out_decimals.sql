DROP VIEW swap_views;

CREATE VIEW swap_views AS
SELECT
  s.id, s.cabal_id, s.source_kind, s.source_id, s.action, s.symbol, s.in_amount, s.out_decimals, s.out_amount, s.status,
  s.failure_code, s.tx_signature, s.created_at, s.confirmed_at,
  (
    s.status = 'failed' AND NOT EXISTS (
      SELECT 1 FROM swaps later
      WHERE later.source_kind = s.source_kind AND later.source_id = s.source_id AND later.in_mint = s.in_mint
        AND (later.created_at, later.id) > (s.created_at, s.id)
    )
  ) AS retryable
FROM swaps s;
