CREATE TABLE call_board_entries (
  board text NOT NULL CHECK (board IN ('people', 'cabals')),
  period text NOT NULL CHECK (period IN ('week', 'month', 'all')),
  period_start date NOT NULL,
  subject_id uuid NOT NULL,
  subject_kind text NOT NULL,
  subject_name text NOT NULL,
  subject_handle text,
  subject_picture_url text,
  subject_created_at timestamptz NOT NULL,
  rank integer,
  right_count integer NOT NULL,
  resolved integer NOT NULL,
  voted integer NOT NULL,
  eligible integer NOT NULL,
  rate_bps integer,
  turnout_bps integer NOT NULL,
  baseline_rank integer,
  movement integer,
  movement_kind text NOT NULL DEFAULT 'new' CHECK (movement_kind IN ('up', 'down', 'same', 'new')),
  last_call jsonb,
  run_id uuid NOT NULL,
  computed_at timestamptz NOT NULL,
  PRIMARY KEY (board, period, subject_id)
);

CREATE INDEX call_board_entries_rank_idx ON call_board_entries (board, period, rank);
CREATE INDEX call_board_entries_unranked_idx ON call_board_entries (board, period, (rank IS NULL), resolved DESC);

CREATE TABLE call_board_runs (
  run_id uuid PRIMARY KEY,
  started_at timestamptz NOT NULL,
  finished_at timestamptz NOT NULL,
  rows_written integer NOT NULL
);

CREATE TABLE call_board_triggers (
  id bigserial PRIMARY KEY,
  reason text NOT NULL,
  created_at timestamptz NOT NULL
);
