ALTER TABLE swaps
  ADD COLUMN lease_holder uuid,
  ADD COLUMN lease_until timestamptz,
  ADD CONSTRAINT swaps_lease_check CHECK ((lease_holder IS NULL) = (lease_until IS NULL));

CREATE TABLE evm_swap_locks (
  cabal_id uuid PRIMARY KEY,
  swap_id uuid NOT NULL REFERENCES swaps (id),
  expires_at timestamptz NOT NULL
);
