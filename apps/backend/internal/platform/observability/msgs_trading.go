package observability

var TradingSwapFinished = Msg{
	Name:     "trading.swap.finished",
	Required: []string{"swap_id", "source", "status", "failure_code"},
}

var TradingSwapForceResolved = Msg{
	Name:     "trading.swap.force_resolved",
	Required: []string{"swap_id", "signature", "from", "to", "reason"},
}

var TradingEngineBlocked = Msg{
	Name:     "trading.engine.blocked",
	Required: []string{"proposal_id", "cabal_id", "code", "have", "need"},
}

var TradingRetryRequested = Msg{
	Name:     "trading.retry.requested",
	Required: []string{"swap_id", "cabal_id", "proposal_id", "requested_by"},
}
