package events

import (
	"testing"

	"github.com/google/uuid"
)

func TestAgentEventsReportTheirTypeAndAggregate(t *testing.T) {
	t.Parallel()
	agent, cabal, proposal, user, intent := uuid.UUID{1}, uuid.UUID{2}, uuid.UUID{3}, uuid.UUID{4}, uuid.UUID{5}
	for _, tt := range []struct {
		event     Event
		wire      string
		aggregate string
		id        uuid.UUID
	}{
		{AgentEnabled{AgentID: agent, CabalID: cabal, ProposalID: proposal}, "agent.enabled", "agent", agent},
		{AgentPaused{AgentID: agent, CabalID: cabal, ProposalID: proposal}, "agent.paused", "agent", agent},
		{AgentRemoved{AgentID: agent, CabalID: cabal, ProposalID: proposal}, "agent.removed", "agent", agent},
		{AgentChangeBlocked{CabalID: cabal, ProposalID: proposal}, "agent.change_blocked", "proposal", proposal},
		{AgentKeyRevealed{AgentID: agent, CabalID: cabal, UserID: user}, "agent.key_revealed", "agent", agent},
		{AgentIntentCreated{IntentID: intent, AgentID: agent}, "agent.intent_created", "agent_intent", intent},
	} {
		if string(tt.event.Type()) != tt.wire || tt.event.AggregateType() != tt.aggregate ||
			tt.event.AggregateID() != tt.id {
			t.Errorf("%T = type %s, aggregate %s %s; want %s, %s %s", tt.event, tt.event.Type(),
				tt.event.AggregateType(), tt.event.AggregateID(), tt.wire, tt.aggregate, tt.id)
		}
	}
}
