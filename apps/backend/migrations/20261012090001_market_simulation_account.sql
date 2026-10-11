CREATE TABLE evm_simulation_account (
  singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
  privy_wallet_id text NOT NULL UNIQUE CHECK (privy_wallet_id <> ''),
  address text NOT NULL CHECK (address ~ '^0x[0-9a-f]{40}$'),
  created_at timestamptz NOT NULL
);
