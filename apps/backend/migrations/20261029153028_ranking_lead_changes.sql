CREATE TABLE call_board_lead_changes (
  id uuid PRIMARY KEY,
  board text NOT NULL CHECK (board IN ('people', 'cabals')),
  period text NOT NULL CHECK (period IN ('week', 'month', 'all')),
  period_start date NOT NULL,
  leader_id uuid NOT NULL,
  passed_id uuid NOT NULL,
  leader_rate_bps integer NOT NULL,
  leader_prior_rank integer,
  movement_places integer NOT NULL,
  call jsonb,
  changed_at timestamptz NOT NULL,
  UNIQUE (board, period, period_start, leader_id, changed_at)
);

CREATE INDEX call_board_lead_changes_latest_idx ON call_board_lead_changes (board, period, changed_at DESC);
