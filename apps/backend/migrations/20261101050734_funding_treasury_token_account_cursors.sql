CREATE TABLE treasury_token_account_cursors (
  cabal_id uuid NOT NULL REFERENCES cabals (id),
  token_account text NOT NULL,
  last_signature text NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (cabal_id, token_account)
);
