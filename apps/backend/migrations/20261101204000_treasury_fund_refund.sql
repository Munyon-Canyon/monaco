ALTER TABLE fund_transfers DROP CONSTRAINT fund_transfers_status_check;
ALTER TABLE fund_transfers ADD CONSTRAINT fund_transfers_status_check
  CHECK (status IN ('created', 'submitted', 'landed', 'settled', 'failed', 'refunded'));

ALTER TABLE fund_transfers
  ADD COLUMN refund_signed_tx bytea,
  ADD COLUMN refund_signature text UNIQUE,
  ADD COLUMN refund_last_valid_block_height bigint,
  ADD COLUMN refund_submitted_at timestamptz,
  ADD COLUMN refunded_at timestamptz;

ALTER TABLE fund_transfers
  ADD CONSTRAINT fund_transfers_refund_stored CHECK (
    (refund_signature IS NULL) = (refund_signed_tx IS NULL)
    AND (refund_signature IS NULL) = (refund_last_valid_block_height IS NULL)
    AND (refund_signature IS NULL) = (refund_submitted_at IS NULL)
  ),
  ADD CONSTRAINT fund_transfers_refund_final CHECK (
    (status = 'refunded') = (refunded_at IS NOT NULL AND refund_signature IS NOT NULL AND landed_at IS NOT NULL)
  ),
  ADD CONSTRAINT fund_transfers_refund_from_landed CHECK (
    refund_signature IS NULL OR status IN ('landed', 'refunded')
  );
