package events

func fundingRegistrations() []Registration {
	return []Registration{
		Register[DepositCredited](TypeDepositCredited, 1),
		Register[CabalPaused](TypeCabalPaused, 1),
		Register[CabalResumed](TypeCabalResumed, 1),
		Register[CabalExternalDepositDetected](TypeCabalExternalDepositDetected, 1),
		Register[OnrampStatusChanged](TypeOnrampStatusChanged, 1),
		Register[WithdrawalSubmitted](TypeWithdrawalSubmitted, 1),
		Register[WithdrawalConfirmed](TypeWithdrawalConfirmed, 1),
		Register[WithdrawalFailed](TypeWithdrawalFailed, 1),
	}
}
