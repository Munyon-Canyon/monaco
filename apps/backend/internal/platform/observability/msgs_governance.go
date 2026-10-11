package observability

var GovernanceBanVoidSkipped = Msg{
	Name: "governance.ban_void.skipped", Required: []string{"proposal_id", "user_id", "code"},
}

var GovernanceProposalComparisonRefreshed = Msg{
	Name: "governance.proposal.comparison_refreshed", Required: []string{"proposal_id", "compared"},
}
