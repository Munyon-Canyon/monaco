CREATE TABLE weekly_ledgers (
  user_id uuid NOT NULL,
  week text NOT NULL,
  window_start timestamptz NOT NULL,
  window_end timestamptz NOT NULL,
  payload jsonb NOT NULL,
  built_at timestamptz NOT NULL,
  PRIMARY KEY (user_id, week)
);

CREATE INDEX weekly_ledgers_week_idx ON weekly_ledgers (week);

CREATE TABLE weekly_ledger_skips (
  user_id uuid NOT NULL,
  week text NOT NULL,
  PRIMARY KEY (user_id, week)
);
