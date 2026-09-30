CREATE TABLE price_points (
  mint text NOT NULL,
  ts timestamptz NOT NULL,
  price_micros bigint NOT NULL,
  source text NOT NULL,
  PRIMARY KEY (mint, ts)
);

CREATE INDEX price_points_ts_idx ON price_points (ts);
