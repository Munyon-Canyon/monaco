CREATE TABLE withdrawals (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id),
  amount_micros numeric(20, 0) NOT NULL CHECK (amount_micros >= 1000000),
  to_address text NOT NULL,
  status text NOT NULL CHECK (status IN ('created', 'submitted', 'confirmed', 'failed')),
  signed_tx bytea,
  tx_signature text UNIQUE,
  last_valid_block_height bigint,
  fail_code text,
  created_at timestamptz NOT NULL,
  submitted_at timestamptz,
  completed_at timestamptz,
  CHECK (status NOT IN ('submitted', 'confirmed')
    OR (signed_tx IS NOT NULL AND tx_signature IS NOT NULL AND last_valid_block_height IS NOT NULL
      AND submitted_at IS NOT NULL)),
  CHECK ((status = 'failed') = (fail_code IS NOT NULL)),
  CHECK ((status IN ('confirmed', 'failed')) = (completed_at IS NOT NULL))
);

CREATE INDEX withdrawals_in_flight ON withdrawals (user_id) WHERE status IN ('created', 'submitted');
CREATE INDEX withdrawals_open ON withdrawals (created_at) WHERE status IN ('created', 'submitted');
