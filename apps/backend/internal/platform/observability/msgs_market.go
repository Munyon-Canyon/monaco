package observability

var (
	MarketBestChainRefreshed = Msg{
		Name:     "market.best_chain.refreshed",
		Required: []string{"symbol", "network", "venue", "duration_ms"},
	}
	MarketBestChainRefreshFailed = Msg{
		Name:     "market.best_chain.refresh_failed",
		Required: []string{"symbol", "error", "code"},
	}
)
