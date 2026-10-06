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
	Required: []string{"user_id", "on_chain_micros", "in_flight_fund_micros", "in_flight_withdrawal_micros"},
}

var FundingPauseChanged = Msg{
	Name:     "funding.pause.changed",
	Required: []string{"scope", "cabal_id", "reasons_before", "reasons_after"},
}

var FundingWatchOwnTransfer = Msg{
	Name:     "funding.watch.own_transfer",
	Required: []string{"cabal_id", "tx"},
}

var FundingWatchIgnored = Msg{
	Name:     "funding.watch.ignored",
	Required: []string{"cabal_id", "tx", "mint", "amount", "outcome"},
}

var FundingWatchDetected = Msg{
	Name:     "funding.watch.detected",
	Required: []string{"external_deposit_id", "cabal_id", "mint", "amount", "status_before", "status_after"},
}

var FundingBounceMoved = Msg{
	Name:     "funding.bounce.moved",
	Required: []string{"external_deposit_id", "cabal_id", "status_before", "status_after"},
}

var FundingBounceFailed = Msg{
	Name:     "funding.bounce.failed",
	Required: []string{"external_deposit_id", "cabal_id", "reason"},
}

var FundingBounceOps = Msg{
	Name:     "funding.bounce.ops",
	Required: []string{"external_deposit_id", "action", "operator"},
}

var FundingReconcileFailed = Msg{
	Name:     "funding.reconcile.failed",
	Required: []string{"cabal_id", "code", "err"},
}
