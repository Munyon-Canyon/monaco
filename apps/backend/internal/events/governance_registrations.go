package events

func governanceRegistrations() []Registration {
	return []Registration{
		Register[ProposalCreated](TypeProposalCreated, 1),
		Register[ProposalPassed](TypeProposalPassed, 1),
		Register[ProposalFailed](TypeProposalFailed, 1),
		Register[ProposalExpired](TypeProposalExpired, 1),
		Register[ProposalWithdrawn](TypeProposalWithdrawn, 1),
		Register[ProposalVoided](TypeProposalVoided, 1),
		Register[ProposalExecuted](TypeProposalExecuted, 1),
		Register[ProposalExecutionBlocked](TypeProposalExecutionBlocked, 1),
		Register[ProposalReopened](TypeProposalReopened, 1),
	}
}
