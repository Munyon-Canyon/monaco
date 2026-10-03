CREATE TABLE cashout_sell_plans (
  job_id uuid PRIMARY KEY,
  cabal_id uuid NOT NULL,
  legs jsonb NOT NULL CHECK (jsonb_typeof(legs) = 'array'),
  created_at timestamptz NOT NULL
);
