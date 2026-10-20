package observability

var MarketCoinGeckoSubMicroDropped = Msg{
	Name:     "market.coingecko.sub_micro_dropped",
	Required: []string{"mint", "days", "dropped"},
}

var MarketReconcileSkippedNoKey = Msg{
	Name: "market.reconcile.skipped_no_key",
}

var MarketHistoryDisabled = Msg{
	Name: "market.history.disabled",
}

var MarketHistoryCooldown = Msg{
	Name:     "market.history.cooldown",
	Required: []string{"poller", "until", "retry_after_s"},
}

var MarketChartHistoryFailed = Msg{
	Name:     "market.chart.history_failed",
	Required: []string{"mint", "code", "err"},
}

var MarketCoinGeckoBudget = Msg{
	Name:     "market.coingecko.budget",
	Required: []string{"percent", "calls", "limit"},
}

var MarketCoinGeckoUsageFailed = Msg{
	Name:     "market.coingecko.usage_failed",
	Required: []string{"err"},
}
