package observability

var (
	MarketDecimalsCorrected = Msg{
		Name:     "market.catalog.decimals_corrected",
		Required: []string{"symbol", "mint", "issuer_decimals", "chain_decimals"},
	}
	MarketMultiplierChanged = Msg{
		Name:     "market.catalog.multiplier_changed",
		Required: []string{"symbol", "mint", "multiplier_before", "multiplier_after"},
	}
)
