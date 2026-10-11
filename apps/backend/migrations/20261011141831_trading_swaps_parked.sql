ALTER TABLE swaps
  ADD COLUMN parked_at timestamptz,
  ADD COLUMN parked_code text,
  ADD CONSTRAINT swaps_parked_check CHECK ((parked_at IS NULL) = (parked_code IS NULL));
