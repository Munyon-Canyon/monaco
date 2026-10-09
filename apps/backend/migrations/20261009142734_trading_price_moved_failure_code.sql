ALTER TABLE swaps
  DROP CONSTRAINT swaps_failure_code_check,
  ADD CONSTRAINT swaps_failure_code_check CHECK (
    failure_code IS NULL OR failure_code IN (
      'never_submitted', 'blockhash_expired', 'jupiter_failed', 'force_resolved', 'source_cancelled', 'price_moved'
    )
  );
