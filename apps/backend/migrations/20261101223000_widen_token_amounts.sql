DROP VIEW swap_views;

ALTER TABLE swaps
  ALTER COLUMN in_amount TYPE numeric(78,0),
  ALTER COLUMN quote_out_amount TYPE numeric(78,0),
  ALTER COLUMN out_amount TYPE numeric(78,0);

ALTER TABLE proposals
  ALTER COLUMN token_amount TYPE numeric(78,0),
  ALTER COLUMN quote_out_amount TYPE numeric(78,0);

ALTER TABLE cabal_txn_entries ALTER COLUMN amount TYPE numeric(78,0);

ALTER TABLE cabal_positions ALTER COLUMN units TYPE numeric(78,0);

ALTER TABLE cabal_activity ALTER COLUMN units TYPE numeric(78,0);

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
  ) AS retryable,
  s.fee_micros
FROM swaps s;
