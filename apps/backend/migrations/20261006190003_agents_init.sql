CREATE TABLE agents (
  id uuid PRIMARY KEY,
  cabal_id uuid NOT NULL,
  name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 40),
  budget_usdc_micros bigint NOT NULL CHECK (budget_usdc_micros > 0),
  status text NOT NULL CHECK (status IN ('active', 'paused', 'removed')),
  added_by_proposal_id uuid NOT NULL UNIQUE,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  removed_at timestamptz
);

CREATE UNIQUE INDEX agents_live_cabal_idx ON agents (cabal_id) WHERE status <> 'removed';

CREATE TABLE agent_keys (
  agent_id uuid PRIMARY KEY REFERENCES agents (id),
  key_hash bytea NOT NULL UNIQUE,
  key_ciphertext bytea,
  created_at timestamptz NOT NULL,
  wiped_at timestamptz
);

CREATE TABLE agent_key_reveals (
  id uuid PRIMARY KEY,
  agent_id uuid NOT NULL REFERENCES agents (id),
  user_id uuid NOT NULL,
  revealed_at timestamptz NOT NULL
);

CREATE TABLE agent_intents (
  id uuid PRIMARY KEY,
  agent_id uuid NOT NULL REFERENCES agents (id),
  cabal_id uuid NOT NULL,
  side text NOT NULL CHECK (side IN ('buy', 'sell')),
  mint text NOT NULL,
  symbol text NOT NULL,
  usdc_micros bigint CHECK (usdc_micros > 0),
  token_amount bigint CHECK (token_amount > 0),
  quote_out_amount bigint NOT NULL,
  reason text CHECK (char_length(reason) <= 280),
  status text NOT NULL CHECK (status IN ('accepted', 'executed', 'rejected', 'failed')),
  reject_code text,
  swap_id uuid,
  tx_signature text,
  filled_usdc_micros bigint,
  filled_token_amount bigint,
  created_at timestamptz NOT NULL,
  settled_at timestamptz,
  CONSTRAINT agent_intents_side_amount_check CHECK (
    (side = 'buy' AND usdc_micros IS NOT NULL AND token_amount IS NULL)
    OR (side = 'sell' AND token_amount IS NOT NULL AND usdc_micros IS NULL)
  )
);

CREATE INDEX agent_intents_agent_status_idx ON agent_intents (agent_id, status);
