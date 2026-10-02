package observability

var MarketPriceMoved = Msg{
	Name:     "market.price_moved",
	Required: []string{"asset", "threshold", "change"},
}
