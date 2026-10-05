ALTER TABLE cash_out_jobs
  ADD COLUMN slice_micros numeric(20,0),
  ADD COLUMN returned_units numeric(20,0) NOT NULL DEFAULT 0 CHECK (returned_units >= 0);
UPDATE cash_out_jobs SET slice_micros = payout_micros;
ALTER TABLE cash_out_jobs
  ALTER COLUMN slice_micros SET NOT NULL,
  ADD CONSTRAINT cash_out_jobs_payout_within_slice CHECK (payout_micros <= slice_micros),
  ADD CONSTRAINT cash_out_jobs_returned_within_units CHECK (returned_units <= share_units);

CREATE TABLE cash_out_sells (
  swap_id uuid PRIMARY KEY,
  job_id uuid NOT NULL REFERENCES cash_out_jobs(id),
  batch_size integer NOT NULL CHECK (batch_size > 0),
  status text NOT NULL CHECK (status IN ('confirmed', 'failed')),
  usdc_out_micros numeric(20,0) NOT NULL CHECK (usdc_out_micros >= 0),
  created_at timestamptz NOT NULL
);
CREATE INDEX cash_out_sells_job ON cash_out_sells (job_id);
