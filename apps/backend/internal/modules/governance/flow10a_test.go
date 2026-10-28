package governance_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow10a_RetractVote_OK(t *testing.T) {
	t.Parallel()
	flows.F10aRetractVoteOK(scenario.New(t, withGovernance(), scenario.WithPostHog(t)))
}

func TestFlow10a_RetractVote_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F10aRetractVoteUnauthorized(scenario.New(t, withGovernance()))
}

func TestFlow10a_RetractVote_ProposalNotFound(t *testing.T) {
	t.Parallel()
	flows.F10aRetractVoteProposalNotFound(scenario.New(t, withGovernance()))
}

func TestFlow10a_RetractVote_NotAVoter(t *testing.T) {
	t.Parallel()
	flows.F10aRetractVoteNotAVoter(scenario.New(t, withGovernance()))
}

func TestFlow10a_RetractVote_ProposalClosed(t *testing.T) {
	t.Parallel()
	flows.F10aRetractVoteProposalClosed(scenario.New(t, withGovernance()))
}

func TestFlow10a_RetractVote_VoteNotFound(t *testing.T) {
	t.Parallel()
	flows.F10aRetractVoteVoteNotFound(scenario.New(t, withGovernance()))
}

func TestFlow10a_RetractVote_UndoExpired(t *testing.T) {
	t.Parallel()
	flows.F10aRetractVoteUndoExpired(scenario.New(t, withGovernance()))
}
