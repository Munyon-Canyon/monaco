CREATE TABLE user_sessions (
  privy_session_id text PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id),
  first_seen_at timestamptz NOT NULL
);

CREATE INDEX user_sessions_user_idx ON user_sessions (user_id);
