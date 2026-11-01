package observability

var FundingEVMForwardMoved = Msg{
	Name:     "funding.evm_forward.moved",
	Required: []string{"forward_id", "user_id", "amount_micros", "status_before", "status_after"},
}

var FundingEVMForwardUnrouted = Msg{
	Name:     "funding.evm_forward.unrouted",
	Required: []string{"execution_id", "wallet_id", "network", "code"},
}

var FundingEVMForwardStuck = Msg{
	Name:     "funding.evm_forward.stuck",
	Required: []string{"forward_id", "user_id", "network", "pending_since"},
}

var FundingEVMForwardArrivingUnavailable = Msg{
	Name:     "funding.evm_forward.arriving_unavailable",
	Required: []string{"user_id", "code"},
}

var FundingEVMForwardFailed = Msg{
	Name:     "funding.evm_forward.failed",
	Required: []string{"forward_id", "user_id", "network", "failed_since"},
}

var FundingEVMDepositToDeletedUser = Msg{
	Name:     "funding.evm_forward.deleted_user",
	Required: []string{"user_id", "execution_id", "network", "amount_micros"},
}
