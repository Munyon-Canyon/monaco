ALTER TABLE trade_steps ADD COLUMN prior_privy_action_ids text[] NOT NULL DEFAULT '{}';

UPDATE trade_steps SET prior_privy_action_ids = ARRAY[prior_privy_action_id] WHERE prior_privy_action_id IS NOT NULL;

-- atlas:nolint destructive
ALTER TABLE trade_steps DROP COLUMN prior_privy_action_id;
