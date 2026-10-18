package observability

var MarketBackfillSkippedNoKey = Msg{
	Name: "market.backfill.skipped_no_key",
}

var MarketBackfillUnlisted = Msg{
	Name:     "market.backfill.coingecko_not_listed",
	Required: []string{"mint"},
}

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
