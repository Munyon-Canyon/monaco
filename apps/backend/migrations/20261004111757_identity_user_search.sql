CREATE INDEX users_handle_pattern_idx ON users (handle text_pattern_ops);
CREATE INDEX users_display_name_trgm_idx ON users USING gin (lower(display_name) gin_trgm_ops);
