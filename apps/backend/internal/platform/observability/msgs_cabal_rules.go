package observability

var CabalRuleChangeSkipped = Msg{
	Name: "cabal.rule_change.skipped", Required: []string{"proposal_id", "reason"},
}
