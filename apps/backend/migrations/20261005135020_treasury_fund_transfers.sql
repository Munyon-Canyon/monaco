CREATE TABLE fund_transfers (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL,
  cabal_id uuid NOT NULL,
  amount_micros numeric(20, 0) NOT NULL CHECK (amount_micros >= 1000000),
  from_address text NOT NULL,
  to_address text NOT NULL,
  status text NOT NULL CHECK (status IN ('created', 'submitted', 'landed', 'settled', 'failed')),
  signed_tx bytea,
  tx_signature text UNIQUE,
  last_valid_block_height bigint,
  share_units numeric(20, 0) CHECK (share_units > 0),
  fail_code text,
  created_at timestamptz NOT NULL,
  submitted_at timestamptz,
  landed_at timestamptz,
  settled_at timestamptz,
  CHECK (status IN ('created', 'failed')
    OR (signed_tx IS NOT NULL AND tx_signature IS NOT NULL AND last_valid_block_height IS NOT NULL
      AND submitted_at IS NOT NULL)),
  CHECK ((status IN ('landed', 'settled')) <= (landed_at IS NOT NULL)),
  CHECK ((status = 'settled') = (settled_at IS NOT NULL AND share_units IS NOT NULL)),
  CHECK ((status = 'failed') = (fail_code IS NOT NULL))
);

CREATE INDEX fund_transfers_in_flight ON fund_transfers (user_id) WHERE status IN ('created', 'submitted');
CREATE INDEX fund_transfers_open ON fund_transfers (status, created_at) WHERE status IN ('created', 'submitted', 'landed');
