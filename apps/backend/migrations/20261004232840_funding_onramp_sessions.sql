CREATE TABLE onramp_sessions (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id),
  token_hash bytea NOT NULL UNIQUE,
  suggested_amount_micros numeric(20, 0) CHECK (suggested_amount_micros > 0),
  cabal_id uuid,
  status text NOT NULL CHECK (
    status IN ('created', 'opened', 'confirmed', 'submitted', 'cancelled', 'failed', 'expired')
  ),
  provider text,
  created_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL,
  opened_at timestamptz,
  completed_at timestamptz
);

CREATE INDEX onramp_sessions_live ON onramp_sessions (created_at) WHERE status IN ('created', 'opened');
