CREATE TABLE asset_price_moves (
  asset_id uuid NOT NULL,
  threshold_bps int NOT NULL,
  trading_day date NOT NULL,
  event_id uuid NOT NULL,
  created_at timestamptz NOT NULL,
  PRIMARY KEY (asset_id, threshold_bps, trading_day)
);
