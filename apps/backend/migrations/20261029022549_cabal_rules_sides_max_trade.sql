ALTER TABLE cabals
  ADD COLUMN show_sides boolean NOT NULL DEFAULT false,
  ADD COLUMN show_sides_changed_at timestamptz,
  ADD COLUMN max_trade_bps integer CHECK (max_trade_bps BETWEEN 100 AND 10000 AND max_trade_bps % 100 = 0);
