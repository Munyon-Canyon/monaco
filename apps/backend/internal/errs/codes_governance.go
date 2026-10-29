package errs

const (
	CodeProposalNotFound   Code = "proposal_not_found"
	CodeProposalClosed     Code = "proposal_closed"
	CodeNotAVoter          Code = "not_a_voter"
	CodeNotProposer        Code = "not_proposer"
	CodeWithdrawNotAllowed Code = "withdraw_not_allowed"
	CodeLiveSwapExists     Code = "live_swap_exists"
	CodePotExceeded        Code = "pot_exceeded"
	CodeProposalStillOpen  Code = "proposal_still_open"
	CodePotCashShort       Code = "pot_cash_short"
	CodeOverTradeCap       Code = "over_trade_cap"
	CodeVoteNotFound       Code = "vote_not_found"
	CodeUndoExpired        Code = "undo_expired"
	CodeRuleUnchanged      Code = "rule_unchanged"
	CodeRuleChangeOpen     Code = "rule_change_open"
)

func (codeFiles) Governance() map[Code]Row {
	return map[Code]Row{
		CodeProposalNotFound: {Name: "ProposalNotFound", Kind: KindNotFound, Message: "That proposal was not found."},
		CodeProposalClosed: {
			Name: "ProposalClosed", Kind: KindBlocked, Message: "Voting on this proposal has closed.",
		},
		CodeNotAVoter: {
			Name: "NotAVoter", Kind: KindForbidden,
			Message: "Only members of the cabal when this proposal opened can vote on it.",
		},
		CodeNotProposer: {
			Name: "NotProposer", Kind: KindForbidden,
			Message: "Only the member who made this proposal can withdraw it.",
		},
		CodeWithdrawNotAllowed: {
			Name: "WithdrawNotAllowed", Kind: KindBlocked,
			Message: "This proposal can no longer be withdrawn.",
		},
		CodeLiveSwapExists: {
			Name: "LiveSwapExists", Kind: KindBlocked, Message: "This proposal's trade is already underway.",
		},
		CodePotExceeded: {Name: "PotExceeded", Kind: KindBlocked, Message: "That amount is more than the cabal holds."},
		CodePotCashShort: {
			Name: "PotCashShort", Kind: KindBlocked,
			Message: "The pot doesn't have that much cash. Sell a holding first or propose less.",
		},
		CodeOverTradeCap: {
			Name: "OverTradeCap", Kind: KindBlocked, Message: "That trade is over this cabal's cap per trade.",
		},
		CodeVoteNotFound: {Name: "VoteNotFound", Kind: KindNotFound, Message: "You haven't voted on this proposal."},
		CodeUndoExpired: {
			Name: "UndoExpired", Kind: KindBlocked,
			Message: "It's too late to undo that vote. You can still change it.",
		},
		CodeRuleUnchanged: {
			Name: "RuleUnchanged", Kind: KindBlocked, Message: "That rule already has that value.",
		},
		CodeRuleChangeOpen: {
			Name: "RuleChangeOpen", Kind: KindConflict,
			Message: "A proposal to change that rule is already open.",
		},
		CodeProposalStillOpen: {
			Name: "ProposalStillOpen", Kind: KindUnavailable, Retryable: true,
			Message: "This proposal is still being decided. Try again in a moment.",
		},
	}
}
