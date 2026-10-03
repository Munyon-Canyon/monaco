package observability

var FundingDepositDuplicate = Msg{
	Name:     "funding.deposit.duplicate",
	Required: []string{"wallet_address"},
}

var FundingDepositCredited = Msg{
	Name:     "funding.deposit.credited",
	Required: []string{"wallet_address", "amount_micros"},
}

var FundingPauseChanged = Msg{
	Name:     "funding.pause.changed",
	Required: []string{"scope", "cabal_id", "reasons_before", "reasons_after"},
}
