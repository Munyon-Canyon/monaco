CREATE TABLE withdrawal_authorizations (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id),
  session_id text NOT NULL,
  amount_micros numeric(20, 0) NOT NULL CHECK (amount_micros >= 1000000),
  destination_kind text NOT NULL CHECK (destination_kind IN ('solana', 'evm')),
  network text NOT NULL CHECK (network IN ('solana', 'base', 'arbitrum', 'ethereum')),
  to_address text NOT NULL,
  created_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL CHECK (expires_at > created_at),
  consumed_at timestamptz,
  consumed_by uuid REFERENCES withdrawals (id),
  CHECK ((consumed_at IS NULL) = (consumed_by IS NULL))
);

CREATE INDEX withdrawal_authorizations_user_idx ON withdrawal_authorizations (user_id, created_at);

CREATE UNIQUE INDEX withdrawal_authorizations_consumed_by_key ON withdrawal_authorizations (consumed_by)
  WHERE consumed_by IS NOT NULL;

CREATE INDEX withdrawals_confirmed_destination ON withdrawals (user_id, network, to_address)
  WHERE status = 'confirmed';
