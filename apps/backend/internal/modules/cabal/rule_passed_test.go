package cabal_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestRulePassed_showSidesChangeEmitsTheCabalUpdatedHintAfterCommit(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	sent := &hintLog{}
	hints := adapters.Hints{Publish: sent}
	on := true
	e := events.CabalUpdated{
		V:       1,
		CabalID: c.ID.UUID(),
		ActorID: c.Creator.ID.UUID(),
		Changes: events.CabalChanges{ShowSides: &on},
	}
	if err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		return hints.Updated(ctx, tx, e, f.clock.Now())
	}); err != nil {
		t.Fatal(err)
	}
	if want := "cabal." + c.ID.String() + ".updated"; len(sent.keys) != 1 || sent.keys[0] != want {
		t.Fatalf("hints = %q, want %q", sent.keys, want)
	}
}
