package events

func fundingRegistrations() []Registration {
	return []Registration{
		Register[DepositCredited](TypeDepositCredited, 1),
		Register[CabalPaused](TypeCabalPaused, 1),
		Register[CabalResumed](TypeCabalResumed, 1),
		Register[OnrampStatusChanged](TypeOnrampStatusChanged, 1),
	}
}
