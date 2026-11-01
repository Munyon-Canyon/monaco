ALTER TABLE swap_operator_actions
  DROP CONSTRAINT swap_operator_actions_action_check,
  ADD CONSTRAINT swap_operator_actions_action_check
    CHECK (action IN ('unpark', 'fail_parked', 'retry_bridge_back', 'return_evm_cash', 'force_resolve'));
