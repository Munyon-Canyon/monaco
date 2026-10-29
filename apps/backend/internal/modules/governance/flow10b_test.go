package governance_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow10b_SetReminder_OK(t *testing.T) {
	t.Parallel()
	flows.F10bSetReminderOK(scenario.New(t, withGovernance()))
}

func TestFlow10b_SetReminder_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F10bSetReminderUnauthorized(scenario.New(t, withGovernance()))
}

func TestFlow10b_SetReminder_ProposalNotFound(t *testing.T) {
	t.Parallel()
	flows.F10bSetReminderProposalNotFound(scenario.New(t, withGovernance()))
}

func TestFlow10b_SetReminder_NotAVoter(t *testing.T) {
	t.Parallel()
	flows.F10bSetReminderNotAVoter(scenario.New(t, withGovernance()))
}

func TestFlow10b_SetReminder_ProposalClosed(t *testing.T) {
	t.Parallel()
	flows.F10bSetReminderProposalClosed(scenario.New(t, withGovernance()))
}

func TestFlow10b_ClearReminder_OK(t *testing.T) {
	t.Parallel()
	flows.F10bClearReminderOK(scenario.New(t, withGovernance()))
}

func TestFlow10b_ClearReminder_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F10bClearReminderUnauthorized(scenario.New(t, withGovernance()))
}

func TestFlow10b_ClearReminder_ProposalNotFound(t *testing.T) {
	t.Parallel()
	flows.F10bClearReminderProposalNotFound(scenario.New(t, withGovernance()))
}

func TestFlow10b_CloseReminderPoller_OK(t *testing.T) {
	t.Parallel()
	flows.F10bCloseReminderPollerOK(scenario.New(t, withGovernance()))
}
