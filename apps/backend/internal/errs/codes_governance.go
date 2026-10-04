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
		CodeProposalStillOpen: {
			Name: "ProposalStillOpen", Kind: KindUnavailable, Retryable: true,
			Message: "This proposal is still being decided. Try again in a moment.",
		},
	}
}
