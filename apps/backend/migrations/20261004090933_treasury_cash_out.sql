CREATE TABLE cash_out_jobs (
  id uuid PRIMARY KEY,
  cabal_id uuid NOT NULL,
  user_id uuid NOT NULL,
  share_units numeric(20,0) NOT NULL CHECK (share_units > 0),
  payout_micros numeric(20,0) NOT NULL CHECK (payout_micros > 0),
  sell_usdc_micros numeric(20,0) NOT NULL DEFAULT 0 CHECK (sell_usdc_micros >= 0),
  status text NOT NULL CHECK (status IN ('started', 'selling', 'paying', 'completed', 'partial', 'failed')),
  result_code text,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX cash_out_jobs_one_live_member
  ON cash_out_jobs (cabal_id, user_id)
  WHERE status NOT IN ('completed', 'partial', 'failed');

CREATE TABLE cash_out_payouts (
  job_id uuid NOT NULL REFERENCES cash_out_jobs(id),
  attempt smallint NOT NULL CHECK (attempt > 0),
  signature text NOT NULL UNIQUE,
  signed_tx bytea NOT NULL,
  status text NOT NULL CHECK (status IN ('signed', 'broadcast', 'confirmed', 'expired')),
  created_at timestamptz NOT NULL,
  PRIMARY KEY (job_id, attempt)
);
