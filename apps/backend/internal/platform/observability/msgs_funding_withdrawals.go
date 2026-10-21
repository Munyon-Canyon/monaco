package observability

var FundingWithdrawalMoved = Msg{
	Name:     "funding.withdrawal.moved",
	Required: []string{"withdrawal_id", "user_id", "amount_micros", "status_before", "status_after"},
}

var FundingWithdrawalBroadcastFailed = Msg{
	Name:     "funding.withdrawal.broadcast_failed",
	Required: []string{"withdrawal_id", "code"},
}

var FundingWithdrawalNeedsSupport = Msg{
	Name:     "funding.withdrawal.needs_support",
	Required: []string{"withdrawal_id", "privy_action_id"},
}

var FundingWithdrawalAuthorized = Msg{
	Name:     "funding.withdrawal.authorized",
	Required: []string{"authorization_id", "user_id", "amount_micros", "network"},
}
