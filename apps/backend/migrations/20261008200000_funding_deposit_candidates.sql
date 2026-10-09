CREATE TABLE deposit_candidates (
  tx_signature text NOT NULL,
  wallet_address text NOT NULL,
  user_id uuid NOT NULL,
  slot bigint NOT NULL,
  block_time timestamptz,
  source text NOT NULL CHECK (source IN ('poller', 'stream', 'recovery')),
  status text NOT NULL CHECK (status IN ('pending', 'credited', 'not_deposit', 'ours')),
  seen_at timestamptz NOT NULL,
  resolved_at timestamptz,
  PRIMARY KEY (tx_signature, wallet_address)
);

CREATE INDEX deposit_candidates_pending_seen_at_idx
  ON deposit_candidates (seen_at) WHERE status = 'pending';
