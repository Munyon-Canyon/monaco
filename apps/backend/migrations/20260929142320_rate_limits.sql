CREATE TABLE rate_limit_buckets (
  key text PRIMARY KEY,
  tokens_milli bigint NOT NULL,
  updated_at timestamptz NOT NULL
);
