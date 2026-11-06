package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func settledTallies() *fakeTallies {
	votes := map[ids.ProposalID]app.Tally{}
	for _, name := range []string{"failed", "expired", "crash", "last", "voided", "withdrawn"} {
		votes[ids.ProposalIDFrom(flows.SettledProposal(name))] = app.Tally{
			Yes:    2,
			No:     4,
			Voters: 6,
			Needed: 4,
			Kind:   "buy",
		}
	}
	return &fakeTallies{votes: votes}
}

func settledScenario(t *testing.T) *scenario.Scenario {
	t.Helper()
	return chatScenarioWith(t, []social.Option{social.WithTallies(settledTallies())})
}

func TestFlow22d_WriteProposalFailedMoment_OK(t *testing.T) {
	t.Parallel()
	flows.F22dWriteProposalFailedMomentOK(settledScenario(t))
}

func TestFlow22d_WriteProposalExpiredMoment_OK(t *testing.T) {
	t.Parallel()
	flows.F22dWriteProposalExpiredMomentOK(settledScenario(t))
}

func TestChatMoments_SilentForRuleVoidedWithdrawnAndUnknownProposals(t *testing.T) {
	t.Parallel()
	flows.F22dWriteProposalFailedMomentSilent(settledScenario(t))
}

func TestFlow22d_WriteMemberJoinedMoment_OK(t *testing.T) {
	t.Parallel()
	flows.F22dWriteMemberJoinedMomentOK(chatScenario(t))
}

func TestFlow22d_WriteCabalCreatedMoment_OK(t *testing.T) {
	t.Parallel()
	flows.F22dWriteCabalCreatedMomentOK(chatScenario(t))
}

func TestFlow22d_WriteTradeFilledMoment_OK(t *testing.T) {
	t.Parallel()
	flows.F22dWriteTradeFilledMomentOK(chatScenario(t))
}
