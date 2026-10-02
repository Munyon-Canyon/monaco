CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX assets_display_name_trgm ON assets USING gin (display_name gin_trgm_ops);
