package cabal_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestSeed_cabalWithCreatorReplaysTheCreateEvents(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	m := cabal.New(module.Deps{Pool: pool, Bus: testkit.NATS(t).Conn, Clock: clock.Real{}})
	seeded := testkit.Seed(t, pool, "cabal-with-creator", m.Consumers()...)
	if len(seeded) != 2 || seeded[0].Actor != "user:01890a5d-ac96-774b-bcce-b302099a805a" {
		t.Fatalf("seeded %+v, want cabal.created and cabal.member_joined by its creator", seeded)
	}
	ev, ok := seeded[0].Event.(events.CabalCreated)
	if !ok || ev.Name != "Friends pot" || ev.CabalID.String() != "01890a5d-ac96-774b-bcce-b302099a8059" {
		t.Fatalf("event = %#v, want the seeded cabal.created", seeded[0].Event)
	}
	if ev.CreatorID.String() != "01890a5d-ac96-774b-bcce-b302099a805a" || ev.TreasuryAddress == "" {
		t.Fatalf("event = %#v, want the creator and a treasury address", ev)
	}
	assertSeededCreatorJoined(t, seeded[1].Event)
}

func assertSeededCreatorJoined(t *testing.T, event any) {
	t.Helper()
	joined, ok := event.(events.CabalMemberJoined)
	if !ok || joined.CabalID.String() != "01890a5d-ac96-774b-bcce-b302099a8059" ||
		joined.UserID.String() != "01890a5d-ac96-774b-bcce-b302099a805a" || joined.Role != "creator" || joined.Via != "create" {
		t.Fatalf("event = %#v, want the creator's seeded cabal.member_joined", event)
	}
}
