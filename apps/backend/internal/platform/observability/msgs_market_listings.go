package observability

var (
	MarketListingSourceMissing = Msg{
		Name:     "market.listings.source_missing",
		Required: []string{"issuer", "symbol", "network"},
	}
	MarketListingVerifyFailed = Msg{
		Name:     "market.listings.verify_failed",
		Required: []string{"issuer", "symbol", "network", "reason"},
	}
)
