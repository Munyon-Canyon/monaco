package observability

var TradingSwapFinished = Msg{
	Name:     "trading.swap.finished",
	Required: []string{"swap_id", "source", "status", "failure_code"},
}
