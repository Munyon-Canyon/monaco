package observability

var TreasuryLedgerPosted = Msg{
	Name:     "treasury.ledger.posted",
	Required: []string{"cabal_id", "asset", "before", "after", "delta"},
}

var TreasuryFundSubmitted = Msg{
	Name:     "treasury.fund.submitted",
	Required: []string{"transfer_id", "cabal_id", "amount_micros", "status_before", "status_after"},
}

var TreasuryFundSignFailed = Msg{
	Name:     "treasury.fund.sign_failed",
	Required: []string{"transfer_id", "code", "status_before", "status_after"},
}

var TreasuryFundBroadcastFailed = Msg{
	Name:     "treasury.fund.broadcast_failed",
	Required: []string{"transfer_id", "code"},
}

var TreasuryFundFailed = Msg{
	Name:     "treasury.fund.failed",
	Required: []string{"transfer_id", "code", "status_before", "status_after"},
}

var TreasuryFundMintWaiting = Msg{
	Name:     "treasury.fund.mint_waiting",
	Required: []string{"transfer_id", "cabal_id"},
}

var TreasuryFundSettled = Msg{
	Name:     "treasury.fund.settled",
	Required: []string{"transfer_id", "cabal_id", "share_units", "status_before", "status_after"},
}

var TreasuryCashOutSaleSettled = Msg{
	Name:     "treasury.cashout.sale_settled",
	Required: []string{"job_id", "cabal_id", "before", "after", "paid_micros", "returned_units"},
}

var TreasuryCashOutMoved = Msg{
	Name:     "treasury.cashout.moved",
	Required: []string{"job_id", "before", "after", "payout_micros"},
}

var TreasuryCashOutBroadcastFailed = Msg{
	Name:     "treasury.cashout.broadcast_failed",
	Required: []string{"job_id", "code"},
}
