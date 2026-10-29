package events

import (
	"testing"

	"github.com/google/uuid"
)

func TestAdminGrantEventsReportTheirTypeAndAggregate(t *testing.T) {
	t.Parallel()
	user := uuid.UUID{3}
	for _, tt := range []struct {
		event Event
		wire  string
	}{
		{AdminGranted{UserID: user}, "admin.granted"},
		{AdminRevoked{UserID: user}, "admin.revoked"},
	} {
		if string(tt.event.Type()) != tt.wire || tt.event.AggregateType() != "admin" || tt.event.AggregateID() != user {
			t.Errorf("%T = type %s, aggregate %s %s; want %s, admin %s", tt.event, tt.event.Type(),
				tt.event.AggregateType(), tt.event.AggregateID(), tt.wire, user)
		}
	}
}

func TestAdminApprovalEventsReportTheirTypeAndAggregate(t *testing.T) {
	t.Parallel()
	approval := uuid.UUID{4}
	for _, tt := range []struct {
		event Event
		wire  string
	}{
		{AdminApprovalRequested{ApprovalID: approval}, "admin.approval_requested"},
		{AdminCabalBanApproved{ApprovalID: approval}, "admin.cabal_ban_approved"},
	} {
		if string(tt.event.Type()) != tt.wire || tt.event.AggregateType() != "admin_approval" ||
			tt.event.AggregateID() != approval {
			t.Errorf("%T = type %s, aggregate %s %s; want %s, admin_approval %s", tt.event, tt.event.Type(),
				tt.event.AggregateType(), tt.event.AggregateID(), tt.wire, approval)
		}
	}
}
