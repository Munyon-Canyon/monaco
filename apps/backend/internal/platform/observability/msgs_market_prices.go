package observability

var (
	MarketPriceMoved = Msg{
		Name:     "market.price_moved",
		Required: []string{"asset", "threshold", "change"},
	}
	MarketPricesMissing    = Msg{Name: "market.prices.missing", Required: []string{"missing", "by_issuer", "mints"}}
	MarketRouteProbeFailed = Msg{
		Name:     "market.route_probe_failed",
		Required: []string{"symbol", "side", "error", "code"},
	}
	JupiterPriceSkipped = Msg{Name: "jupiter.price_skipped", Required: []string{"mint", "usd_price", "stock_price"}}
)
