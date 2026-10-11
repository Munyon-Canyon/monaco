ALTER TABLE trade_steps
  ADD COLUMN attempt smallint NOT NULL DEFAULT 1 CHECK (attempt >= 1),
  ADD COLUMN prior_privy_action_id text;

ALTER TABLE swap_operator_actions
  DROP CONSTRAINT swap_operator_actions_action_check,
  ADD CONSTRAINT swap_operator_actions_action_check
    CHECK (action IN ('unpark', 'fail_parked', 'retry_bridge_back'));
