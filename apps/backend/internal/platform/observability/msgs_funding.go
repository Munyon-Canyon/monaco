package observability

var FundingDepositDuplicate = Msg{
	Name:     "funding.deposit.duplicate",
	Required: []string{"wallet_address"},
}

var FundingDepositCredited = Msg{
	Name:     "funding.deposit.credited",
	Required: []string{"wallet_address", "amount_micros"},
}
