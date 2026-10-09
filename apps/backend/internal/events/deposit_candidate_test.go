package events_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestDepositCandidateEvents_aggregateOnTheCandidate(t *testing.T) {
	t.Parallel()
	id := testkit.NewIDs(1).NewV7()
	for _, tt := range []struct {
		event events.Event
		wire  string
	}{
		{events.DepositCandidateSeen{CandidateID: id}, "deposit.candidate_seen"},
		{events.DepositCandidateDismissed{CandidateID: id}, "deposit.candidate_dismissed"},
	} {
		if string(tt.event.Type()) != tt.wire || tt.event.AggregateType() != "deposit" ||
			tt.event.AggregateID() != id {
			t.Errorf("%T = %s on %s %s, want %s on deposit %s", tt.event, tt.event.Type(),
				tt.event.AggregateType(), tt.event.AggregateID(), tt.wire, id)
		}
	}
}
