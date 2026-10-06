package social_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestSeed_cabalWithChatHoldsThreeMembersAnOutsiderAndAThread(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	m := cabal.New(module.Deps{Pool: pool, Bus: testkit.NATS(t).Conn, Clock: clock.Real{}})
	seeded := testkit.Seed(t, pool, "cabal-with-chat", m.Consumers()...)
	outsider, ok := seeded[0].Event.(events.UserCreated)
	created, isCabal := seeded[1].Event.(events.CabalCreated)
	if len(seeded) != 10 || !ok || !isCabal {
		t.Fatalf("seeded %+v, want the outsider, a cabal, three joins and five messages", seeded)
	}
	members := seededMembers(t, created.CabalID, outsider.UserID, seeded[2:5])
	top, replies := map[uuid.UUID]bool{}, map[uuid.UUID]int{}
	for _, s := range seeded[5:] {
		posted := s.Event.(events.ChatMessagePosted)
		if posted.CabalID != created.CabalID || !members[posted.AuthorID] ||
			s.Actor != "user:"+posted.AuthorID.String() {
			t.Fatalf("message %+v by %s, want one posted in the seeded cabal by a member", posted, s.Actor)
		}
		if posted.ParentID == nil {
			top[posted.MessageID] = true
			continue
		}
		if !top[*posted.ParentID] {
			t.Fatalf("reply %s names parent %s, want an earlier top-level message", posted.MessageID, *posted.ParentID)
		}
		replies[*posted.ParentID]++
	}
	if len(top) != 3 || len(replies) != 1 {
		t.Fatalf("top-level %d, threads %v, want three top-level messages and one thread", len(top), replies)
	}
}

func seededMembers(t *testing.T, cabalID, outsider uuid.UUID, joins []testkit.Seeded) map[uuid.UUID]bool {
	t.Helper()
	members := map[uuid.UUID]bool{}
	for _, s := range joins {
		joined, ok := s.Event.(events.CabalMemberJoined)
		if !ok || joined.CabalID != cabalID || joined.UserID == outsider {
			t.Fatalf("event = %#v, want a member other than the outsider joining the seeded cabal", s.Event)
		}
		members[joined.UserID] = true
	}
	return members
}
