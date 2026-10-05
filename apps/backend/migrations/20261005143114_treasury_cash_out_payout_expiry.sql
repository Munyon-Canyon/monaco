ALTER TABLE cash_out_payouts
  ADD COLUMN last_valid_block_height bigint NOT NULL CHECK (last_valid_block_height >= 0),
  DROP CONSTRAINT cash_out_payouts_status_check,
  ADD CONSTRAINT cash_out_payouts_status_check
    CHECK (status IN ('signed', 'broadcast', 'confirmed', 'expired', 'failed'));
CREATE UNIQUE INDEX cash_out_payouts_one_live ON cash_out_payouts (job_id)
  WHERE status IN ('signed', 'broadcast', 'confirmed');
