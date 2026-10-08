CREATE TABLE user_blocks (
  id uuid PRIMARY KEY,
  blocker_id uuid NOT NULL,
  blocked_id uuid NOT NULL,
  created_at timestamptz NOT NULL,
  UNIQUE (blocker_id, blocked_id),
  CHECK (blocker_id <> blocked_id)
);

CREATE INDEX user_blocks_blocked_idx ON user_blocks (blocked_id);

CREATE TABLE reports (
  id uuid PRIMARY KEY,
  reporter_id uuid NOT NULL,
  kind text NOT NULL CHECK (kind IN ('message', 'comment', 'user', 'cabal')),
  target_id uuid NOT NULL,
  reason text NOT NULL CHECK (reason IN ('spam', 'abuse', 'other')),
  note text CHECK (char_length(note) <= 500),
  status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
  created_at timestamptz NOT NULL
);

CREATE UNIQUE INDEX reports_open_once_idx ON reports (reporter_id, kind, target_id) WHERE status = 'open';

CREATE INDEX reports_open_idx ON reports (created_at, id) WHERE status = 'open';
