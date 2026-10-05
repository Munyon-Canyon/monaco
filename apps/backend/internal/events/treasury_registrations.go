package events

func treasuryRegistrations() []Registration {
	return []Registration{Register[CashOutStarted](TypeCashOutStarted, 1)}
}
