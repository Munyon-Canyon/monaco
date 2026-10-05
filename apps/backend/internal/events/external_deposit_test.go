package events_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestCabalExternalDepositDetected_isKeyedByItsCabal(t *testing.T) {
	t.Parallel()
	cabal := testkit.NewIDs(1).NewV7()
	ev := events.CabalExternalDepositDetected{CabalID: cabal}
	if ev.Type() != events.TypeCabalExternalDepositDetected || ev.AggregateType() != "cabal" ||
		ev.AggregateID() != cabal {
		t.Fatalf("event = %s/%s/%s, want %s/cabal/%s", ev.Type(), ev.AggregateType(), ev.AggregateID(),
			events.TypeCabalExternalDepositDetected, cabal)
	}
}
