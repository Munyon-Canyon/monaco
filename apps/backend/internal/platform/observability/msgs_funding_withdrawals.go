package observability

var FundingWithdrawalMoved = Msg{
	Name:     "funding.withdrawal.moved",
	Required: []string{"withdrawal_id", "user_id", "amount_micros", "status_before", "status_after"},
}

var FundingWithdrawalBroadcastFailed = Msg{
	Name:     "funding.withdrawal.broadcast_failed",
	Required: []string{"withdrawal_id", "code"},
}
