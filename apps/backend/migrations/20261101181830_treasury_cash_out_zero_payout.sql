ALTER TABLE cash_out_jobs
  DROP CONSTRAINT cash_out_jobs_payout_micros_check,
  ADD CONSTRAINT cash_out_jobs_payout_micros_check CHECK (payout_micros >= 0);
