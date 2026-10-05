CREATE TABLE price_backfills (
  mint text PRIMARY KEY,
  requested_at timestamptz NOT NULL,
  done_at timestamptz,
  last_code text
);
