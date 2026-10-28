CREATE TABLE price_day_ranges (
  mint text NOT NULL,
  day date NOT NULL,
  high_micros bigint NOT NULL CHECK (high_micros > 0),
  low_micros bigint NOT NULL CHECK (low_micros > 0),
  source text NOT NULL CHECK (source IN ('samples', 'history')),
  PRIMARY KEY (mint, day)
);
