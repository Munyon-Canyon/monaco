package observability

var FundingDepositDuplicate = Msg{
	Name:     "funding.deposit.duplicate",
	Required: []string{"wallet_address"},
}

var FundingDepositCredited = Msg{
	Name:     "funding.deposit.credited",
	Required: []string{"wallet_address", "amount_micros"},
}

var FundingBalanceClamped = Msg{
	Name:     "funding.balance.clamped",
	Required: []string{"user_id", "on_chain_micros", "in_flight_fund_micros"},
}
