package governance_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow13_WithdrawProposal_OK(t *testing.T) {
	t.Parallel()
	flows.F13WithdrawProposalOK(scenario.New(t, withGovernance()))
}

func TestFlow13_WithdrawProposal_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F13WithdrawProposalUnauthorized(scenario.New(t, withGovernance()))
}

func TestFlow13_WithdrawProposal_ProposalNotFound(t *testing.T) {
	t.Parallel()
	flows.F13WithdrawProposalProposalNotFound(scenario.New(t, withGovernance()))
}

func TestFlow13_WithdrawProposal_NotProposer(t *testing.T) {
	t.Parallel()
	flows.F13WithdrawProposalNotProposer(scenario.New(t, withGovernance()))
}

func TestFlow13_WithdrawProposal_ProposalClosed(t *testing.T) {
	t.Parallel()
	flows.F13WithdrawProposalProposalClosed(scenario.New(t, withGovernance()))
}

func TestFlow13_WithdrawProposal_WithdrawNotAllowed(t *testing.T) {
	t.Parallel()
	flows.F13WithdrawProposalWithdrawNotAllowed(scenario.New(t, withGovernance()))
}

func TestFlow13a_VoidProposal_OK(t *testing.T) {
	t.Parallel()
	flows.F13aVoidProposalOK(scenario.New(t, withGovernance()))
}

func TestFlow13a_VoidProposal_ProposalClosed(t *testing.T) {
	t.Parallel()
	flows.F13aVoidProposalProposalClosed(scenario.New(t, withGovernance()))
}

func TestFlow13a_VoidProposal_LiveSwapExists(t *testing.T) {
	t.Parallel()
	flows.F13aVoidProposalLiveSwapExists(scenario.New(t, withGovernance()))
}
