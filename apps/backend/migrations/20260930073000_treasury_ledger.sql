CREATE TABLE cabal_txns (
  id uuid PRIMARY KEY,
  cabal_id uuid NOT NULL,
  kind text NOT NULL CHECK (kind IN ('swap', 'fund', 'cash_out')),
  status text NOT NULL CHECK (status IN ('pending', 'settled', 'failed')),
  swap_id uuid,
  transfer_id uuid,
  tx_signature text,
  created_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX cabal_txns_swap_id ON cabal_txns (swap_id) WHERE swap_id IS NOT NULL;
CREATE INDEX cabal_txns_cabal_created ON cabal_txns (cabal_id, created_at);
CREATE INDEX cabal_txns_transfer_id ON cabal_txns (transfer_id);

CREATE TABLE cabal_txn_entries (
  txn_id uuid NOT NULL REFERENCES cabal_txns (id),
  seq smallint NOT NULL,
  account text NOT NULL CHECK (account IN ('treasury', 'venue', 'members', 'fees')),
  asset text NOT NULL CHECK (asset <> ''),
  amount bigint NOT NULL CHECK (amount <> 0),
  PRIMARY KEY (txn_id, seq)
);

CREATE TABLE user_txns (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL,
  cabal_id uuid,
  kind text NOT NULL CHECK (kind IN ('deposit', 'withdrawal', 'fund', 'cash_out')),
  status text NOT NULL CHECK (status IN ('pending', 'settled', 'failed')),
  transfer_id uuid,
  tx_signature text,
  created_at timestamptz NOT NULL
);
CREATE INDEX user_txns_user_created ON user_txns (user_id, created_at);
CREATE INDEX user_txns_cabal_created ON user_txns (cabal_id, created_at);
CREATE INDEX user_txns_transfer_id ON user_txns (transfer_id);

CREATE TABLE user_txn_entries (
  txn_id uuid NOT NULL REFERENCES user_txns (id),
  seq smallint NOT NULL,
  account text NOT NULL CHECK (account IN ('wallet', 'external', 'cabal', 'holder', 'issuer')),
  asset text NOT NULL CHECK (asset <> ''),
  amount bigint NOT NULL CHECK (amount <> 0),
  PRIMARY KEY (txn_id, seq)
);

CREATE TABLE cabal_positions (
  cabal_id uuid NOT NULL,
  asset text NOT NULL,
  units numeric(20,0) NOT NULL CHECK (units >= 0),
  cost_basis_micros numeric(20,0) NOT NULL CHECK (cost_basis_micros >= 0),
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (cabal_id, asset)
);

CREATE TABLE user_positions (
  user_id uuid NOT NULL,
  cabal_id uuid NOT NULL,
  share_units numeric(20,0) NOT NULL CHECK (share_units >= 0),
  contributed_micros numeric(20,0) NOT NULL CHECK (contributed_micros >= 0),
  withdrawn_micros numeric(20,0) NOT NULL CHECK (withdrawn_micros >= 0),
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (user_id, cabal_id)
);
