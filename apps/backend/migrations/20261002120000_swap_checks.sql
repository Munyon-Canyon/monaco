ALTER TABLE swaps
  ADD CONSTRAINT swaps_status_check CHECK (status IN ('created', 'submitted', 'confirmed', 'failed')),
  ADD CONSTRAINT swaps_source_kind_check CHECK (source_kind IN ('proposal', 'cashout')),
  ADD CONSTRAINT swaps_failure_code_check CHECK (
    failure_code IS NULL OR failure_code IN (
      'never_submitted', 'blockhash_expired', 'jupiter_failed', 'force_resolved', 'source_cancelled'
    )
  );

CREATE INDEX swaps_source_idx ON swaps (source_kind, source_id, in_mint, created_at, id);
