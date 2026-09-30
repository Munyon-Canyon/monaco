CREATE TABLE proposals (
  id uuid PRIMARY KEY,
  cabal_id uuid NOT NULL,
  proposer_id uuid NOT NULL,
  kind text NOT NULL,
  symbol text NOT NULL,
  mint text NOT NULL,
  usdc_micros bigint,
  token_amount bigint,
  thesis text CHECK (char_length(thesis) <= 280),
  quote_out_amount bigint NOT NULL,
  status text NOT NULL,
  status_reason text,
  void_reason text,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  CONSTRAINT proposals_one_amount_check CHECK (
    (usdc_micros IS NOT NULL AND usdc_micros > 0 AND token_amount IS NULL)
    OR (token_amount IS NOT NULL AND token_amount > 0 AND usdc_micros IS NULL)
  )
);

CREATE INDEX proposals_open_expiry_idx ON proposals (status, expires_at) WHERE status = 'open';

CREATE INDEX proposals_cabal_created_idx ON proposals (cabal_id, created_at DESC, id DESC);

CREATE TABLE proposal_voters (
  proposal_id uuid NOT NULL REFERENCES proposals,
  voter_id uuid NOT NULL,
  PRIMARY KEY (proposal_id, voter_id)
);

CREATE TABLE votes (
  proposal_id uuid NOT NULL REFERENCES proposals,
  voter_id uuid NOT NULL,
  choice text NOT NULL CHECK (choice IN ('yes', 'no')),
  cast_at timestamptz NOT NULL,
  PRIMARY KEY (proposal_id, voter_id)
);
