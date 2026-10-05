package events_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestHintKeys(t *testing.T) {
	t.Parallel()
	id := testkit.NewIDs(9).NewV7()
	if got, want := events.CabalActivityChangedHint(
		ids.CabalIDFrom(id),
	), "cabal."+id.String()+".activity_changed"; got != want {
		t.Fatalf("CabalActivityChangedHint = %q, want %q", got, want)
	}
	if got, want := events.UserBalanceChangedHint(
		ids.UserIDFrom(id),
	), "user."+id.String()+".balance_changed"; got != want {
		t.Fatalf("UserBalanceChangedHint = %q, want %q", got, want)
	}
}
