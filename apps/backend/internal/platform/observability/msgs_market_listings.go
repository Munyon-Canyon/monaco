package observability

var (
	MarketListingSourceMissing = Msg{
		Name:     "market.listings.source_missing",
		Required: []string{"issuer", "symbol", "network"},
	}
	MarketListingAssetSkipped = Msg{
		Name:     "market.listings.asset_skipped",
		Required: []string{"issuer", "symbol", "reason"},
	}
	MarketListingVerifyFailed = Msg{
		Name:     "market.listings.verify_failed",
		Required: []string{"issuer", "symbol", "network", "reason"},
	}
)
