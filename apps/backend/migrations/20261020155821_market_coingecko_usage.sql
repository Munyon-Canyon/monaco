CREATE TABLE coingecko_usage (
  month date PRIMARY KEY CHECK (month = date_trunc('month', month)::date),
  calls integer NOT NULL CHECK (calls >= 0),
  updated_at timestamptz NOT NULL
);
