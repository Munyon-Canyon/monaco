package observability

var TreasuryLedgerPosted = Msg{
	Name:     "treasury.ledger.posted",
	Required: []string{"cabal_id", "asset", "before", "after", "delta"},
}

var TreasuryCashOutSaleSettled = Msg{
	Name:     "treasury.cashout.sale_settled",
	Required: []string{"job_id", "cabal_id", "before", "after", "paid_micros", "returned_units"},
}
