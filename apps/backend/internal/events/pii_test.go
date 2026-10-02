package events_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
)

type eventType interface {
	Type() events.Type
}

func TestRegisteredEvents_tagPersonalFieldNames(t *testing.T) {
	t.Parallel()
	personal := []string{
		"user_id", "voter_ids", "handle", "email", "phone", "wallet_address", "address", "creator_id", "actor_id",
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
		walkPersonalTags(t, entry.Type, typ, personal)
		delete(walked, entry.Type)
	}
	for typ := range walked {
		t.Errorf("the personal-field walk lists %s, which is not registered", typ)
	}
}

func walkPersonalTags(t *testing.T, event events.Type, root reflect.Type, personal []string) {
	t.Helper()
	eventsPkg := reflect.TypeFor[events.SystemPinged]().PkgPath()
	seen := map[reflect.Type]bool{}
	var walk func(reflect.Type)
	walk = func(typ reflect.Type) {
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		if typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array || typ.Kind() == reflect.Map {
			walk(typ.Elem())
			return
		}
		if typ.Kind() != reflect.Struct || typ.PkgPath() != eventsPkg || seen[typ] {
			return
		}
		seen[typ] = true
		for i := range typ.NumField() {
			field := typ.Field(i)
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if slices.Contains(personal, name) && field.Tag.Get("pii") != "true" {
				t.Errorf("%s: %s.%s is %q without the pii tag", event, typ.Name(), field.Name, name)
			}
			walk(field.Type)
		}
	}
	walk(root)
}
