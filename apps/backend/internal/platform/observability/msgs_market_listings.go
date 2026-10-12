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
	MarketRouterFallback = Msg{
		Name:     "market.router.fallback",
		Required: []string{"symbol", "code"},
	}
	MarketRouterSolanaUnquoted = Msg{
		Name:     "market.router.solana_unquoted",
		Required: []string{"stock_id", "venue"},
	}
	MarketRouterVenueDropped = Msg{
		Name:     "market.router.venue_dropped",
		Required: []string{"listing_id", "reason", "err", "duration_ms"},
	}
	MarketRouterUniswapSteps = Msg{
		Name: "market.router.uniswap_steps",
		Required: []string{
			"network", "route", "total_ms", "get_code_ms",
			"default_wait_ms", "default_quote_ms", "default_penalize_ms", "default_call_ms",
			"hookless_wait_ms", "hookless_quote_ms", "hookless_penalize_ms", "hookless_call_ms",
		},
	}
	MarketListingVerifyFailed = Msg{
		Name:     "market.listings.verify_failed",
		Required: []string{"issuer", "symbol", "network", "reason"},
	}
)
