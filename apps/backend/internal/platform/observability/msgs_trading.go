package observability

var TradingSwapFinished = Msg{
	Name:     "trading.swap.finished",
	Required: []string{"swap_id", "source", "status", "failure_code"},
}

var TradingSwapForceResolved = Msg{
	Name:     "trading.swap.force_resolved",
	Required: []string{"swap_id", "signature", "from", "to", "reason"},
}
