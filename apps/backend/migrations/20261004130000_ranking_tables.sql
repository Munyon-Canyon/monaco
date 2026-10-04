CREATE TABLE leaderboard_runs (
  run_id uuid PRIMARY KEY,
  as_of timestamptz NOT NULL,
  prices_as_of timestamptz NOT NULL,
  started_at timestamptz NOT NULL,
  finished_at timestamptz NOT NULL,
  rows_written int NOT NULL,
  cabals_excluded int NOT NULL,
  rev int NOT NULL DEFAULT 0
);

CREATE TABLE leaderboard_entries (
  board text NOT NULL,
  range text NOT NULL,
  rank int NOT NULL,
  subject_id uuid NOT NULL,
  subject_name text NOT NULL,
  subject_handle text,
  subject_picture_url text,
  subject_created_at timestamptz NOT NULL,
  value_micros bigint NOT NULL,
  pnl_micros bigint NOT NULL,
  return_bps bigint,
  prices_as_of timestamptz NOT NULL,
  computed_at timestamptz NOT NULL,
  flags text[] NOT NULL DEFAULT '{}',
  PRIMARY KEY (board, range, rank)
);

CREATE INDEX leaderboard_entries_subject ON leaderboard_entries (board, range, subject_id);

CREATE TABLE cabal_value_snapshots (
  cabal_id uuid NOT NULL,
  at timestamptz NOT NULL,
  value_micros bigint NOT NULL,
  nav_per_share_micros bigint NOT NULL,
  total_shares bigint NOT NULL,
  PRIMARY KEY (cabal_id, at)
);

CREATE TABLE ranking_triggers (
  id bigserial PRIMARY KEY,
  cabal_id uuid NOT NULL,
  reason text NOT NULL,
  created_at timestamptz NOT NULL
);
