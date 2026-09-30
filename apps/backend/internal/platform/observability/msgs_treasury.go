package observability

var TreasuryLedgerPosted = Msg{
	Name:     "treasury.ledger.posted",
	Required: []string{"cabal_id", "asset", "before", "after", "delta"},
}
