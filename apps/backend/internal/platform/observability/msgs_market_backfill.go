package observability

var MarketBackfillSkippedNoKey = Msg{
	Name: "market.backfill.skipped_no_key",
}

var MarketCoinGeckoSubMicroDropped = Msg{
	Name:     "market.coingecko.sub_micro_dropped",
	Required: []string{"mint", "days", "dropped"},
}

var MarketReconcileSkippedNoKey = Msg{
	Name: "market.reconcile.skipped_no_key",
}
