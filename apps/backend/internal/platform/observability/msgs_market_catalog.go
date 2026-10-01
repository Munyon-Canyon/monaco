package observability

var MarketDecimalsCorrected = Msg{
	Name:     "market.catalog.decimals_corrected",
	Required: []string{"symbol", "mint", "issuer_decimals", "chain_decimals"},
}
