package observability

var FundingDepositDuplicate = Msg{
	Name:     "funding.deposit.duplicate",
	Required: []string{"wallet_address"},
}

var FundingDepositCredited = Msg{
	Name:     "funding.deposit.credited",
	Required: []string{"wallet_address", "amount_micros"},
}

var FundingCandidateRecorded = Msg{
	Name:     "funding.candidate.recorded",
	Required: []string{"wallet_address"},
}

var FundingCandidateResolved = Msg{
	Name:     "funding.candidate.resolved",
	Required: []string{"wallet_address", "amount_micros"},
}

var FundingCandidateDismissed = Msg{
	Name:     "funding.candidate.dismissed",
	Required: []string{"wallet_address", "reason"},
}

var FundingCandidateUnresolved = Msg{
	Name:     "funding.candidate.unresolved",
	Required: []string{"wallet_address", "code"},
}

var FundingBalanceClamped = Msg{
	Name:     "funding.balance.clamped",
	Required: []string{"user_id", "on_chain_micros", "in_flight_fund_micros"},
}
