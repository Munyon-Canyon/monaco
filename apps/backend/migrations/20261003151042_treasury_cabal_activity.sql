CREATE TABLE cabal_activity (
  id uuid PRIMARY KEY,
  cabal_id uuid NOT NULL,
  kind text NOT NULL CHECK (kind IN ('buy', 'sell', 'fund', 'cash_out')),
  status text NOT NULL CHECK (status IN ('pending', 'confirmed', 'failed')),
  actor_user_id uuid,
  asset text,
  usdc_micros numeric(20,0) CHECK (usdc_micros >= 0),
  units numeric(20,0) CHECK (units >= 0),
  tx_signature text,
  occurred_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);
CREATE INDEX cabal_activity_page ON cabal_activity (cabal_id, occurred_at DESC, id DESC);
