package observability

var TradingSwapFinished = Msg{
	Name:     "trading.swap.finished",
	Required: []string{"swap_id", "source", "status", "failure_code"},
}

var TradingSwapRefused = Msg{
	Name:     "trading.swap.refused",
	Required: []string{"swap_id", "step", "code"},
}

var TradingSwapFillUnread = Msg{
	Name:     "trading.swap.fill_unread",
	Required: []string{"swap_id", "code"},
}

var TradingSwapForceResolved = Msg{
	Name:     "trading.swap.force_resolved",
	Required: []string{"swap_id", "signature", "from", "to", "reason"},
}

var TradingRouteFellBack = Msg{
	Name:     "trading.route.fell_back",
	Required: []string{"proposal_id", "from_listing", "to_listing"},
}

var TradingEngineBlocked = Msg{
	Name:     "trading.engine.blocked",
	Required: []string{"proposal_id", "cabal_id", "code", "have", "need"},
}

var TradingCashOutSellBlocked = Msg{
	Name:     "trading.cashout_sell.blocked",
	Required: []string{"job_id", "cabal_id", "code", "final"},
}

var TradingCashOutPlanShort = Msg{
	Name:     "trading.cashout.plan_short",
	Required: []string{"job_id", "cabal_id", "needed_micros", "planned_micros"},
}

var TradingRetryRequested = Msg{
	Name:     "trading.retry.requested",
	Required: []string{"swap_id", "cabal_id", "proposal_id", "requested_by"},
}

var TradingEngineStubbed = Msg{
	Name:     "trading.engine.stubbed",
	Required: []string{"proposal_id", "kind", "symbol", "usdc_micros"},
}

var TradingCashOutSellStubbed = Msg{
	Name:     "trading.cashout_sell.stubbed",
	Required: []string{"job_id", "cabal_id", "usdc_micros"},
}

var TradingStepChanged = Msg{
	Name: "trading.step.changed",
	Required: []string{
		"swap_id", "cabal_id", "step", "network", "before_status", "status", "amount_micros", "fee_micros",
	},
}

var TradingRoutedSwapParked = Msg{
	Name:     "trading.routed_swap.parked",
	Required: []string{"swap_id", "cabal_id", "code", "step"},
}

var TradingBridgeBackRetried = Msg{
	Name:     "trading.bridge_back.retried",
	Required: []string{"swap_id", "cabal_id", "attempt", "actor", "reason"},
}

var TradingEVMCashReturnQueued = Msg{
	Name: "trading.evm_cash_return.queued",
	Required: []string{
		"swap_id", "requested_swap_id", "cabal_id", "network", "landed_micros", "on_chain_micros", "amount_micros",
		"actor", "reason",
	},
}

var TradingRoutedSwapResolved = Msg{
	Name:     "trading.routed_swap.resolved",
	Required: []string{"swap_id", "cabal_id", "action", "actor", "reason", "parked_code"},
}

var TradingEVMArrivalsChunk = Msg{
	Name:     "trading.evm_arrivals.chunk",
	Required: []string{"network", "owner", "from", "to", "chunk", "rpc_message"},
}

var TradingBridgeOutcomeUnknown = Msg{
	Name:     "trading.bridge.outcome_unknown",
	Required: []string{"swap_id", "cabal_id", "step", "network", "action_id"},
}
