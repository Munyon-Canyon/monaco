CREATE TABLE deposits (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id),
  wallet_address text NOT NULL,
  tx_signature text NOT NULL,
  amount_micros numeric(20, 0) NOT NULL CHECK (amount_micros > 0),
  slot bigint NOT NULL,
  block_time timestamptz,
  credited_at timestamptz NOT NULL,
  UNIQUE (tx_signature, wallet_address)
);

CREATE TABLE deposit_cursors (
  wallet_address text PRIMARY KEY,
  last_signature text,
  cursor_slot bigint NOT NULL DEFAULT 0,
  scanned_at timestamptz NOT NULL
);
