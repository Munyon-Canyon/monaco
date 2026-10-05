package events

func tradingRegistrations() []Registration {
	return []Registration{
		Register[TradeBlocked](TypeTradeBlocked, 1),
		Register[TradeSubmitted](TypeTradeSubmitted, 1),
		Register[TradeConfirmed](TypeTradeConfirmed, 1),
		Register[TradeFailed](TypeTradeFailed, 1),
		Register[TradeRetryRequested](TypeTradeRetryRequested, 1),
	}
}
