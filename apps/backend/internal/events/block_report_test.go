package events

import (
	"testing"

	"github.com/google/uuid"
)

func TestBlockAndReportEventsReportTheirTypeAndAggregate(t *testing.T) {
	t.Parallel()
	block, report := uuid.UUID{1}, uuid.UUID{2}
	for _, tt := range []struct {
		event     Event
		wire      string
		aggregate string
		id        uuid.UUID
	}{
		{BlockCreated{BlockID: block}, "block.created", "user_block", block},
		{BlockRemoved{BlockID: block}, "block.removed", "user_block", block},
		{ReportCreated{ReportID: report}, "report.created", "report", report},
	} {
		if string(tt.event.Type()) != tt.wire || tt.event.AggregateType() != tt.aggregate ||
			tt.event.AggregateID() != tt.id {
			t.Errorf("%T = type %s, aggregate %s %s; want %s, %s %s", tt.event, tt.event.Type(),
				tt.event.AggregateType(), tt.event.AggregateID(), tt.wire, tt.aggregate, tt.id)
		}
	}
}
