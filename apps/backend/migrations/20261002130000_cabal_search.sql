CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX cabals_name_trgm_idx ON cabals USING gin (lower(name) gin_trgm_ops);
