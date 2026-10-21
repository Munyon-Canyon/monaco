ALTER TABLE idempotency_keys ADD COLUMN claim_generation bigint NOT NULL DEFAULT 1 CHECK (claim_generation > 0);
