ALTER TABLE user_wallets
  ADD COLUMN evm_privy_wallet_id text UNIQUE,
  ADD COLUMN evm_address text UNIQUE CHECK (evm_address ~ '^0x[0-9a-f]{40}$'),
  ADD COLUMN evm_created_at timestamptz,
  ADD CONSTRAINT user_wallets_evm_all_or_none CHECK (
    (evm_privy_wallet_id IS NULL) = (evm_address IS NULL)
    AND (evm_address IS NULL) = (evm_created_at IS NULL)
  );

CREATE TABLE treasury_evm_wallets (
  cabal_id uuid PRIMARY KEY REFERENCES cabals (id),
  privy_wallet_id text NOT NULL UNIQUE,
  address text NOT NULL UNIQUE CHECK (address ~ '^0x[0-9a-f]{40}$'),
  created_at timestamptz NOT NULL
);

CREATE TABLE asset_listings (
  id uuid PRIMARY KEY,
  asset_id uuid REFERENCES assets (id),
  issuer text NOT NULL CHECK (issuer IN ('xstocks', 'ondo', 'coinbase', 'robinhood')),
  symbol text NOT NULL,
  network text NOT NULL CHECK (network IN ('solana', 'base', 'arbitrum', 'ethereum', 'robinhood')),
  chain_id bigint,
  token_address text NOT NULL,
  wrapper_address text,
  decimals smallint CHECK (decimals BETWEEN 0 AND 36),
  shares_per_token numeric NOT NULL CHECK (shares_per_token > 0),
  venue text NOT NULL CHECK (venue IN ('jupiter', 'uniswap')),
  enabled bool NOT NULL,
  verified_at timestamptz,
  created_at timestamptz NOT NULL,
  UNIQUE (issuer, network, token_address),
  CHECK ((network = 'solana') = (chain_id IS NULL)),
  CHECK (network = 'solana' OR token_address ~ '^0x[0-9a-f]{40}$'),
  CHECK (wrapper_address IS NULL OR wrapper_address ~ '^0x[0-9a-f]{40}$'),
  CHECK (verified_at IS NULL OR decimals IS NOT NULL),
  CHECK (NOT enabled OR verified_at IS NOT NULL)
);
CREATE INDEX asset_listings_asset ON asset_listings (asset_id) WHERE asset_id IS NOT NULL;
CREATE INDEX asset_listings_symbol ON asset_listings (issuer, symbol);

ALTER TABLE cabal_positions ADD COLUMN listing_id uuid REFERENCES asset_listings (id);

CREATE TABLE evm_deposit_forwards (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id),
  network text NOT NULL CHECK (network IN ('base', 'arbitrum', 'ethereum')),
  source_tx text NOT NULL,
  privy_execution_id text NOT NULL UNIQUE,
  privy_action_id text,
  amount_micros numeric(20, 0) NOT NULL CHECK (amount_micros > 0),
  status text NOT NULL CHECK (status IN ('pending', 'arrived', 'failed')),
  solana_signature text UNIQUE,
  created_at timestamptz NOT NULL,
  arrived_at timestamptz,
  CHECK ((status = 'arrived') = (arrived_at IS NOT NULL AND solana_signature IS NOT NULL))
);
CREATE INDEX evm_deposit_forwards_user_pending ON evm_deposit_forwards (user_id) WHERE status = 'pending';

CREATE TABLE trade_routes (
  trade_id uuid PRIMARY KEY REFERENCES swaps (id),
  comparison jsonb NOT NULL,
  winner_listing_id uuid NOT NULL REFERENCES asset_listings (id),
  created_at timestamptz NOT NULL
);

CREATE TABLE trade_steps (
  id uuid PRIMARY KEY,
  trade_id uuid NOT NULL REFERENCES swaps (id),
  step text NOT NULL CHECK (step IN ('bridge_out', 'swap', 'bridge_back')),
  network text NOT NULL CHECK (network IN ('solana', 'base', 'arbitrum', 'ethereum', 'robinhood')),
  privy_action_id text UNIQUE,
  amount_micros numeric(20, 0) NOT NULL CHECK (amount_micros > 0),
  status text NOT NULL CHECK (status IN ('pending', 'submitted', 'succeeded', 'failed')),
  error text,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (trade_id, step),
  CHECK ((status = 'failed') = (error IS NOT NULL))
);

CREATE TABLE treasury_in_transit (
  cabal_id uuid NOT NULL,
  trade_id uuid NOT NULL,
  network text NOT NULL CHECK (network IN ('base', 'arbitrum', 'ethereum', 'robinhood')),
  amount_micros numeric(20, 0) NOT NULL CHECK (amount_micros > 0),
  created_at timestamptz NOT NULL,
  PRIMARY KEY (trade_id, network)
);
CREATE INDEX treasury_in_transit_cabal ON treasury_in_transit (cabal_id);

ALTER TABLE withdrawals
  ADD COLUMN destination_kind text NOT NULL DEFAULT 'solana' CHECK (destination_kind IN ('solana', 'evm')),
  ADD COLUMN network text NOT NULL DEFAULT 'solana' CHECK (network IN ('solana', 'base', 'arbitrum', 'ethereum')),
  ADD COLUMN evm_address text CHECK (evm_address ~ '^0x[0-9a-f]{40}$'),
  ADD COLUMN privy_action_id text UNIQUE,
  ADD CONSTRAINT withdrawals_destination_matches CHECK (
    (destination_kind = 'solana') = (network = 'solana')
    AND (destination_kind = 'evm') = (evm_address IS NOT NULL)
    AND (evm_address IS NULL OR lower(to_address) = evm_address)
  ),
  DROP CONSTRAINT withdrawals_check,
  ADD CONSTRAINT withdrawals_submitted_has_tx CHECK (status NOT IN ('submitted', 'confirmed') OR submitted_at IS NOT NULL
    AND CASE destination_kind
      WHEN 'solana' THEN signed_tx IS NOT NULL AND tx_signature IS NOT NULL AND last_valid_block_height IS NOT NULL
      ELSE privy_action_id IS NOT NULL
    END);
