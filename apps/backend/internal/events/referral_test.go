package events

import (
	"testing"

	"github.com/google/uuid"
)

func TestReferralEventsUseTheReferralAggregate(t *testing.T) {
	t.Parallel()
	var id uuid.UUID
	for _, event := range []Event{ReferralAttributed{ReferralID: id}, ReferralQualified{ReferralID: id}} {
		if event.AggregateType() != "referral" || event.AggregateID() != id {
			t.Fatalf("%T aggregate = %s %s", event, event.AggregateType(), event.AggregateID())
		}
	}
	attributed, qualified := (ReferralAttributed{}).Type(), (ReferralQualified{}).Type()
	if attributed != TypeReferralAttributed || qualified != TypeReferralQualified {
		t.Fatal("referral event types are wrong")
	}
}
