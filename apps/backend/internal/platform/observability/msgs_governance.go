package observability

var GovernanceBanVoidSkipped = Msg{
	Name: "governance.ban_void.skipped", Required: []string{"proposal_id", "user_id", "code"},
}

var GovernanceProposalComparisonRefreshed = Msg{
	Name: "governance.proposal.comparison_refreshed", Required: []string{"proposal_id", "compared"},
}

var GovernanceProposalComparisonUnlisted = Msg{
	Name: "governance.proposal.comparison_unlisted", Required: []string{"proposal_id", "network"},
}

var GovernanceStatsOverflow = Msg{
	Name: "governance.stats_overflow", Required: []string{"proposal_id", "stat"},
}

var GovernancePassedPriceUnavailable = Msg{
	Name: "governance.passed_price.unavailable", Required: []string{"proposal_id", "code"},
}

var GovernanceBlockedTradeRetried = Msg{
	Name: "governance.blocked_trade.retried", Required: []string{"proposal_id", "cabal_id", "reason"},
}
