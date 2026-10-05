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
