package cabal_test

import (
	"slices"
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

func TestSeed_cabalWithMembersReplaysARequestCabalWithThreeMembers(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	m := cabal.New(module.Deps{Pool: pool, Bus: testkit.NATS(t).Conn, Clock: clock.Real{}})
	seeded := testkit.Seed(t, pool, "cabal-with-members", m.Consumers()...)
	created, ok := seeded[0].Event.(events.CabalCreated)
	if len(seeded) != 4 || !ok || created.JoinMode != "request" {
		t.Fatalf("seeded %+v, want a request cabal.created and three joins", seeded)
	}
	vias := make([]string, 0, 3)
	for _, s := range seeded[1:] {
		joined, ok := s.Event.(events.CabalMemberJoined)
		if !ok || joined.CabalID != created.CabalID || s.Actor != "user:"+joined.UserID.String() {
			t.Fatalf("event = %#v by %s, want a join into the seeded cabal by its member", s.Event, s.Actor)
		}
		vias = append(vias, joined.Via)
	}
	if !slices.Equal(vias, []string{"create", "open", "open"}) {
		t.Fatalf("joins via %q, want the creator then two open joins", vias)
	}
}

func TestSeed_requestCabalWithPendingReplaysOnePendingRequest(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	m := cabal.New(module.Deps{Pool: pool, Bus: testkit.NATS(t).Conn, Clock: clock.Real{}})
	seeded := testkit.Seed(t, pool, "request-cabal-with-pending", m.Consumers()...)
	created, ok := seeded[0].Event.(events.CabalCreated)
	if len(seeded) != 3 || !ok || created.JoinMode != "request" {
		t.Fatalf("seeded %+v, want a request cabal.created, its creator's join and a request", seeded)
	}
	asked, ok := seeded[2].Event.(events.CabalAccessRequested)
	if !ok || asked.CabalID != created.CabalID || asked.Direction != "request" || asked.ActorID != asked.UserID {
		t.Fatalf("event = %#v, want a pending request into the seeded cabal", seeded[2].Event)
	}
}
