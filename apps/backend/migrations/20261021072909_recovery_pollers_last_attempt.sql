ALTER TABLE fund_transfers ADD COLUMN last_attempt_at timestamptz;
ALTER TABLE withdrawals ADD COLUMN last_attempt_at timestamptz;

CREATE INDEX fund_transfers_open_turn ON fund_transfers (last_attempt_at NULLS FIRST, created_at, id)
  WHERE status IN ('submitted', 'landed');
CREATE INDEX withdrawals_open_evm_turn ON withdrawals (last_attempt_at NULLS FIRST, created_at, id)
  WHERE status IN ('created', 'submitted') AND destination_kind = 'evm';
