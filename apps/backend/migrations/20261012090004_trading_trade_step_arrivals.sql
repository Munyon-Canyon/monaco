ALTER TABLE trade_steps
  ADD COLUMN arrived_micros bigint CHECK (arrived_micros > 0 AND arrived_micros <= amount_micros),
  ADD COLUMN arrival_tx text,
  ADD CONSTRAINT trade_steps_arrival_check CHECK ((arrived_micros IS NULL) = (arrival_tx IS NULL));

CREATE UNIQUE INDEX trade_steps_arrival_tx_key ON trade_steps (network, arrival_tx)
WHERE arrival_tx IS NOT NULL AND privy_action_id NOT LIKE 'reused:%';
