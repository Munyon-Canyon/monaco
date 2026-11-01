package errs

const (
	CodeSwapNotFound     Code = "swap_not_found"
	CodeSwapNotStuck     Code = "swap_not_stuck"
	CodeSwapNotRetryable Code = "swap_not_retryable"
	CodeSlippageExceeded Code = "slippage_exceeded"
	CodeNoRoute          Code = "no_route"
	CodeSwapFailed       Code = "swap_failed"
	CodePriceMoved       Code = "price_moved"
	CodeTradeNotStarted  Code = "trade_not_started"
	CodeSwapNotSent      Code = "swap_not_sent"
	CodeCabalSharesShort Code = "cabal_shares_short"
	CodeTradeTooLarge    Code = "trade_too_large"

	CodePinnedChainUnavailable Code = "pinned_chain_unavailable"
	CodeChainTradingOff        Code = "chain_trading_off"
)

func (codeFiles) Trading() map[Code]Row {
	return map[Code]Row{
		CodeSwapNotFound: {Name: "SwapNotFound", Kind: KindNotFound, Message: "That trade was not found."},
		CodeSwapNotStuck: {
			Name: "SwapNotStuck", Kind: KindBlocked, Message: "This trade is no longer awaiting resolution.",
		},
		CodeSwapNotRetryable: {
			Name: "SwapNotRetryable", Kind: KindBlocked,
			Message: "This trade can't be retried. Only the latest failed trade can be.",
		},
		CodeSlippageExceeded: {
			Name: "SlippageExceeded", Kind: KindBlocked,
			Message: "The price moved past the cabal's slippage limit.",
		},
		CodeNoRoute: {
			Name: "NoRoute", Kind: KindBlocked,
			Message: "No route for this trade right now. Try a smaller amount.",
		},
		CodeSwapFailed: {Name: "SwapFailed", Kind: KindBlocked, Message: "The trade did not go through."},
		CodePriceMoved: {
			Name: "PriceMoved", Kind: KindBlocked,
			Message: "The price moved past the cabal's limit since the vote.",
		},
		CodeTradeNotStarted: {
			Name: "TradeNotStarted", Kind: KindBlocked,
			Message: "This trade couldn't start. The money is still in the pot.",
		},
		CodeSwapNotSent: {
			Name: "SwapNotSent", Kind: KindBlocked, Message: "The trade couldn't be sent.",
		},
		CodeCabalSharesShort: {
			Name: "CabalSharesShort", Kind: KindBlocked, Message: "The cabal doesn't hold that many shares.",
		},
		CodeTradeTooLarge: {
			Name: "TradeTooLarge", Kind: KindBlocked,
			Message: "This trade is too large to settle. Lower the amount.",
		},
		CodePinnedChainUnavailable: {
			Name: "PinnedChainUnavailable", Kind: KindBlocked,
			Message: "No route on the chosen chain right now. The money is still in the pot.",
		},
		CodeChainTradingOff: {
			Name: "ChainTradingOff", Kind: KindBlocked,
			Message: "Trading on the chosen chain isn't open yet. The money is still in the pot.",
		},
	}
}

func SwapFailureMessage(failureCode string) string {
	if failureCode == string(CodePriceMoved) {
		return Message(CodePriceMoved)
	}
	if failureCode == "never_submitted" {
		return Message(CodeSwapNotSent)
	}
	return Message(CodeSwapFailed)
}
