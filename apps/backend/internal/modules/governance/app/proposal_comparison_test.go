package app

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestProposalComparison_sweepKeepsYoungAndBusyEntriesAndEvictsExpiredOnes(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(5207)
	proposal := func() ids.ProposalID { return ids.ProposalIDFrom(g.NewV7()) }
	c := NewProposalComparisons(nil, nil)
	start := time.Unix(1_790_000_000, 0)
	unquoted, quoted, busy := proposal(), proposal(), proposal()

	created := c.entry(unquoted, start)
	c.entry(quoted, start).live.QuotedAt = start
	c.entry(proposal(), start)
	if c.entry(unquoted, start) != created {
		t.Fatal("a new proposal's sweep evicted an entry its creator had not quoted yet")
	}

	later := start.Add(comparisonTTL)
	held := c.entry(busy, later)
	held.mu.Lock()
	c.entry(proposal(), later.Add(comparisonTTL))
	held.mu.Unlock()
	if _, ok := c.cached[busy]; !ok {
		t.Fatal("the sweep evicted an entry that was refreshing")
	}
	for _, id := range []ids.ProposalID{unquoted, quoted} {
		if _, ok := c.cached[id]; ok {
			t.Fatalf("the sweep kept expired entry %s", id)
		}
	}
	if len(c.cached) != 2 {
		t.Fatalf("cached %d entries, want the busy one and the new one", len(c.cached))
	}
}

func TestUnquotedPin_listsAnUnlistedPinnedChainWithNoRouteOnItsVenue(t *testing.T) {
	t.Parallel()
	for n, venue := range map[chain.Network]multichain.Venue{
		chain.NetworkSolana: multichain.VenueJupiter, chain.NetworkBase: multichain.VenueUniswap,
	} {
		got := unquotedPin(&multichain.Comparison{}, nil, n)
		want := multichain.UnavailableVenue{Network: n, Venue: venue, Reason: multichain.UnavailableNoRoute}
		if len(got.Ranked) != 0 || !got.Winner.IsZero() || len(got.Unavailable) != 1 || got.Unavailable[0] != want {
			t.Errorf("%s: unquotedPin = %+v, want only %+v unavailable", n, got, want)
		}
	}
}
