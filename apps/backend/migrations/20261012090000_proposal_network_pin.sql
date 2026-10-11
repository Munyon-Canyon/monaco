ALTER TABLE proposals ADD COLUMN network text NULL
  CHECK (network IN ('solana', 'base', 'arbitrum', 'ethereum', 'robinhood'));

ALTER TABLE trade_routes ADD COLUMN pinned_network text NULL
  CHECK (pinned_network IN ('solana', 'base', 'arbitrum', 'ethereum', 'robinhood'));
