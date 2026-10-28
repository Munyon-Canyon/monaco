CREATE TABLE calls (
  proposal_id uuid PRIMARY KEY,
  cabal_id uuid NOT NULL,
  kind text CHECK (kind IN ('buy', 'sell')),
  symbol text,
  mint text,
  asset_id uuid,
  opened_at timestamptz,
  decided_at timestamptz,
  outcome text CHECK (outcome IN ('passed', 'failed', 'expired')),
  market_ref_micros bigint,
  market_ref_observed_at timestamptz,
  fill_price_micros bigint,
  filled_at timestamptz,
  ref_price_micros bigint,
  ref_source text CHECK (ref_source IN ('fill', 'market')),
  resolve_at timestamptz,
  resolved_at timestamptz,
  end_price_micros bigint,
  move_bps integer,
  void_reason text,
  state text NOT NULL CHECK (state IN ('open', 'decided', 'resolved', 'void')),
  updated_at timestamptz NOT NULL
);

CREATE INDEX calls_state_resolve_idx ON calls (state, resolve_at);

CREATE INDEX calls_cabal_resolved_idx ON calls (cabal_id, resolved_at);

CREATE INDEX calls_opened_idx ON calls (opened_at);

CREATE TABLE call_voters (
  proposal_id uuid NOT NULL,
  user_id uuid NOT NULL,
  cabal_id uuid NOT NULL,
  choice text CHECK (choice IN ('yes', 'no')),
  choice_at timestamptz,
  "right" boolean,
  PRIMARY KEY (proposal_id, user_id)
);

CREATE INDEX call_voters_user_idx ON call_voters (user_id, proposal_id);

CREATE TABLE cabal_calls (
  proposal_id uuid PRIMARY KEY,
  cabal_id uuid NOT NULL,
  side text NOT NULL CHECK (side IN ('yes', 'no')),
  "right" boolean
);
