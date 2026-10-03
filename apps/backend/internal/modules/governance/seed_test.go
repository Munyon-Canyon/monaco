package governance_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestSeed_cabalWithOpenProposalReplaysThreeMembersAndOneOpenBuy(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	m := cabal.New(module.Deps{Pool: pool, Bus: testkit.NATS(t).Conn, Clock: clock.Real{}})
	seeded := testkit.Seed(t, pool, "cabal-with-open-proposal", m.Consumers()...)
	if len(seeded) != 5 {
		t.Fatalf("seeded %d events, want a cabal, three joins and a proposal", len(seeded))
	}
	opened := time.Date(2026, 3, 1, 12, 5, 0, 0, time.UTC)
	created, _ := seeded[0].Event.(events.CabalCreated)
	proposed, _ := seeded[4].Event.(events.ProposalCreated)
	type summary struct {
		threshold, kind      string
		sameCabal, byCreator bool
		voters               int
		usdc                 uint64
		expiry               time.Duration
	}
	got := summary{
		threshold: created.Threshold, kind: proposed.Kind, sameCabal: proposed.CabalID == created.CabalID,
		byCreator: proposed.ProposerID == created.CreatorID, voters: proposed.VoterCount,
		usdc: proposed.USDCMicros.Uint64(), expiry: proposed.ExpiresAt.Sub(opened),
	}
	if want := (summary{"majority", "buy", true, true, 3, 5_000_000, 24 * time.Hour}); got != want {
		t.Fatalf("seeded %+v, want %+v", got, want)
	}
	for _, s := range seeded[1:4] {
		if joined, ok := s.Event.(events.CabalMemberJoined); !ok || joined.CabalID != created.CabalID {
			t.Fatalf("event = %#v, want a join into the seeded cabal", s.Event)
		}
	}
}
