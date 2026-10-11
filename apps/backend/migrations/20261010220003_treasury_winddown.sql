ALTER TABLE cash_out_jobs
  ADD COLUMN cause text NOT NULL DEFAULT 'member' CHECK (cause IN ('member', 'wind_down'));
ALTER TABLE cash_out_jobs
  DROP CONSTRAINT cash_out_jobs_payout_micros_check,
  ADD CONSTRAINT cash_out_jobs_payout_micros_check CHECK (payout_micros > 0 OR cause = 'wind_down');
CREATE TABLE cabal_winddowns (
  cabal_id uuid PRIMARY KEY,
  status text NOT NULL CHECK (status IN ('running', 'completed')),
  attempts int NOT NULL DEFAULT 0,
  started_at timestamptz NOT NULL,
  completed_at timestamptz
);
