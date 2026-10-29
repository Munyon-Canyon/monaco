CREATE TABLE call_board_baselines (
  board text NOT NULL,
  period text NOT NULL,
  period_start date NOT NULL,
  day date NOT NULL,
  subject_id uuid NOT NULL,
  rank integer NOT NULL,
  PRIMARY KEY (board, period, day, subject_id)
);

CREATE TABLE call_board_closed (
  board text NOT NULL,
  period text NOT NULL CHECK (period IN ('week', 'month')),
  period_start date NOT NULL,
  rank integer,
  subject_id uuid NOT NULL,
  rate_bps integer,
  right_count integer NOT NULL,
  resolved integer NOT NULL,
  voted integer NOT NULL,
  eligible integer NOT NULL,
  closed_at timestamptz NOT NULL,
  PRIMARY KEY (board, period, period_start, subject_id)
);

CREATE INDEX call_board_closed_subject_idx ON call_board_closed (board, period, subject_id, rank);
