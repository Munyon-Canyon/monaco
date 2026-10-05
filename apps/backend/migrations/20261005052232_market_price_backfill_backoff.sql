ALTER TABLE price_backfills
  ADD COLUMN attempts integer NOT NULL DEFAULT 0,
  ADD COLUMN last_attempt_at timestamptz;
