CREATE TABLE relayer_sol_topups (
  id uuid PRIMARY KEY,
  wallet text NOT NULL,
  lamports numeric(20, 0) NOT NULL CHECK (lamports > 0),
  signature text NOT NULL UNIQUE,
  last_valid_block_height numeric(20, 0) NOT NULL CHECK (last_valid_block_height > 0),
  status text NOT NULL CHECK (status IN ('pending', 'confirmed', 'expired')),
  created_at timestamptz NOT NULL,
  resolved_at timestamptz,
  CHECK ((status = 'pending') = (resolved_at IS NULL))
);

CREATE INDEX relayer_sol_topups_wallet_created ON relayer_sol_topups (wallet, created_at DESC);
