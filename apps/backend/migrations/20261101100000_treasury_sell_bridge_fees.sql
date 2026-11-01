CREATE TABLE sell_bridge_fees (
  trade_id uuid PRIMARY KEY,
  fee_micros numeric(20,0) NOT NULL CHECK (fee_micros >= 0)
);
