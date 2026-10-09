package errs

const (
	CodeSwapNotFound     Code = "swap_not_found"
	CodeSwapNotStuck     Code = "swap_not_stuck"
	CodeSwapNotRetryable Code = "swap_not_retryable"
	CodeSlippageExceeded Code = "slippage_exceeded"
	CodeNoRoute          Code = "no_route"
	CodeSwapFailed       Code = "swap_failed"
	CodeTradeNotStarted  Code = "trade_not_started"
	CodeCabalSharesShort Code = "cabal_shares_short"
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
		CodeTradeNotStarted: {
			Name: "TradeNotStarted", Kind: KindBlocked,
			Message: "This trade couldn't start. The money is still in the pot.",
		},
		CodeCabalSharesShort: {
			Name: "CabalSharesShort", Kind: KindBlocked, Message: "The cabal doesn't hold that many shares.",
		},
	}
}
