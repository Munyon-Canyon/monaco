package events_test

import (
	"reflect"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
)

func TestRegisteredEvents_tagPersonalFieldNames(t *testing.T) {
	t.Parallel()
	personal := []string{
		"user_id", "voter_ids", "handle", "email", "phone", "wallet_address", "address",
		"creator_id", "actor_id", "proposer_id",
	}
	walked := events.GoTypes()
	eventsPkg := reflect.TypeFor[events.SystemPinged]().PkgPath()
	for _, entry := range events.Catalog() {
		typ, ok := walked[entry.Type]
		if !ok {
			t.Errorf("Catalog has %s, which the personal-field walk does not", entry.Type)
			continue
		}
		for _, line := range personalTagViolations(typ, personal, eventsPkg) {
			t.Errorf("%s: %s", entry.Type, line)
		}
	}
}
