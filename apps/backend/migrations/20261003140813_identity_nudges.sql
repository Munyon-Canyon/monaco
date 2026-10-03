ALTER TABLE users
  ADD COLUMN last_nudged_at timestamptz,
  ADD COLUMN nudge_count smallint NOT NULL DEFAULT 0;

CREATE INDEX users_nudge_due_idx ON users (auth_state, last_nudged_at)
  WHERE auth_state IN ('AWAITING_PHONE', 'AWAITING_SOCIALS') AND deleted_at IS NULL;
