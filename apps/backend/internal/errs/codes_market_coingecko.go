package errs

const CodeCoinGeckoRateLimited Code = "coin_gecko_rate_limited"

func (codeFiles) MarketCoingecko() map[Code]Row {
	return map[Code]Row{
		CodeCoinGeckoRateLimited: {
			Name: "CoinGeckoRateLimited", Kind: KindUnavailable, Retryable: true,
			Message: "Price history is busy. Try again shortly.",
		},
	}
}
