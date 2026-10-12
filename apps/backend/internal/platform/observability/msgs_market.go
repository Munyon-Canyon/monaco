package observability

var MarketBestChainRefreshed = Msg{
	Name:     "market.best_chain.refreshed",
	Required: []string{"symbol", "network", "venue", "duration_ms"},
}
