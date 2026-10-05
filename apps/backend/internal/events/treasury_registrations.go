package events

func treasuryRegistrations() []Registration {
	return []Registration{
		Register[CashOutStarted](TypeCashOutStarted, 1),
		Register[FundSubmitted](TypeFundSubmitted, 1),
		Register[Funded](TypeFunded, 1),
		Register[FundFailed](TypeFundFailed, 1),
		Register[CashOutCompleted](TypeCashOutCompleted, 1),
		Register[CashOutFailed](TypeCashOutFailed, 1),
	}
}
