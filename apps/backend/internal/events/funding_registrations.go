package events

func fundingRegistrations() []Registration {
	return []Registration{
		Register[DepositCredited](TypeDepositCredited, 1),
		Register[OnrampStatusChanged](TypeOnrampStatusChanged, 1),
	}
}
