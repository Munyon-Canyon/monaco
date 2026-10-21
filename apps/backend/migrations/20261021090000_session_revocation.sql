ALTER TABLE users ADD COLUMN sessions_valid_after timestamptz;

CREATE TABLE revoked_sessions (
  privy_session_id text PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id),
  revoked_at timestamptz NOT NULL
);

CREATE INDEX revoked_sessions_user_idx ON revoked_sessions (user_id);
