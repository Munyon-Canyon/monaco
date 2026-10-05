CREATE TABLE external_deposits (
  id uuid PRIMARY KEY,
  signature text NOT NULL UNIQUE,
  cabal_id uuid NOT NULL REFERENCES cabals (id),
  sender text NOT NULL,
  mint text NOT NULL,
  asset_id uuid,
  amount numeric(20, 0) NOT NULL CHECK (amount > 0),
  source text NOT NULL CHECK (source IN ('webhook', 'reconcile')),
  status text NOT NULL CHECK (status IN (
    'detected', 'bouncing', 'returned', 'bounce_failed', 'held', 'ignored_dust', 'ignored_unknown')),
  return_address text,
  bounce_signature text UNIQUE,
  bounce_signed_tx bytea,
  bounce_attempts int NOT NULL DEFAULT 0 CHECK (bounce_attempts >= 0),
  detected_at timestamptz NOT NULL,
  resolved_at timestamptz,
  CHECK ((status IN ('ignored_dust', 'ignored_unknown')) <= (resolved_at IS NOT NULL))
);

CREATE INDEX external_deposits_open ON external_deposits (cabal_id) WHERE status IN ('detected', 'bouncing');

CREATE TABLE treasury_watch_cursors (
  cabal_id uuid PRIMARY KEY REFERENCES cabals (id),
  last_signature text NOT NULL,
  updated_at timestamptz NOT NULL
);

ALTER TABLE cabal_pauses ADD COLUMN external_deposit_id uuid REFERENCES external_deposits (id);
ALTER TABLE cabal_pauses ADD CONSTRAINT cabal_pauses_external_deposit_reason
  CHECK (external_deposit_id IS NULL OR reason = 'external_deposit');
