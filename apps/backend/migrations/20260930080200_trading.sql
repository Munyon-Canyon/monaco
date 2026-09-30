CREATE TABLE swaps (
  id uuid PRIMARY KEY,
  source_kind text NOT NULL,
  source_id uuid NOT NULL,
  cabal_id uuid NOT NULL,
  treasury_address text NOT NULL,
  action text NOT NULL CHECK (action IN ('buy', 'sell')),
  symbol text NOT NULL,
  in_mint text NOT NULL,
  out_mint text NOT NULL,
  in_amount bigint NOT NULL,
  quote_out_amount bigint,
  out_amount bigint,
  fee_micros bigint,
  slippage_bps int NOT NULL,
  status text NOT NULL,
  failure_code text,
  execute_request_id text UNIQUE,
  signed_tx bytea,
  tx_signature text UNIQUE,
  created_at timestamptz NOT NULL,
  submitted_at timestamptz,
  confirmed_at timestamptz,
  failed_at timestamptz,
  updated_at timestamptz NOT NULL
);

CREATE UNIQUE INDEX swaps_live_source_idx ON swaps (source_kind, source_id, in_mint) WHERE status <> 'failed';

CREATE INDEX swaps_unfinished_idx ON swaps (status, created_at) WHERE status IN ('created', 'submitted');
