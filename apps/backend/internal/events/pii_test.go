package events_test

import (
	"reflect"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
)

type eventType interface {
	Type() events.Type
}

func TestRegisteredEvents_tagPersonalFieldNames(t *testing.T) {
	t.Parallel()
	personal := []string{
		"user_id", "voter_ids", "handle", "email", "phone", "wallet_address", "address",
		"creator_id", "actor_id", "proposer_id",
	}
	samples := []eventType{
		events.SystemPinged{},
		events.TradeBlocked{},
		events.TradeSubmitted{},
		events.TradeConfirmed{},
		events.TradeFailed{},
		events.ProposalCreated{},
		events.ProposalPassed{},
		events.ProposalFailed{},
		events.ProposalExpired{},
		events.ProposalWithdrawn{},
		events.ProposalVoided{},
		events.ProposalExecuted{},
		events.ProposalExecutionBlocked{},
		events.CabalCreated{},
		events.CabalMemberJoined{},
		events.CabalAccessRequested{},
		events.CabalAccessDecided{},
		events.CabalMemberLeft{},
		events.CabalUpdated{},
		events.PriceTick{},
		events.UserCreated{},
		events.UserAuthStateChanged{},
		events.UserProfileUpdated{},
	}
	walked := make(map[events.Type]reflect.Type, len(samples))
	for _, sample := range samples {
		walked[sample.Type()] = reflect.TypeOf(sample)
	}
	for _, entry := range events.Catalog() {
		typ, ok := walked[entry.Type]
		if !ok {
			t.Errorf("Catalog has %s, which the personal-field walk does not", entry.Type)
			continue
		}
		eventsPkg := reflect.TypeFor[events.SystemPinged]().PkgPath()
		for _, line := range personalTagViolations(typ, personal, eventsPkg) {
			t.Errorf("%s: %s", entry.Type, line)
		}
		delete(walked, entry.Type)
	}
	for typ := range walked {
		t.Errorf("the personal-field walk lists %s, which is not registered", typ)
	}
}
