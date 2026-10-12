package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestProposalComparison_sweepKeepsYoungAndBusyEntriesAndEvictsExpiredOnes(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(5207)
	proposal := func() ids.ProposalID { return ids.ProposalIDFrom(g.NewV7()) }
	c := NewProposalComparisons(nil, nil, nil)
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

type fakePinListings struct {
	id    multichain.ListingID
	venue multichain.Venue
	found bool
	err   error
	asked []chain.Network
}

func (f *fakePinListings) PinListing(
	_ context.Context, _ multichain.StockID, n chain.Network,
) (multichain.ListingID, multichain.Venue, bool, error) {
	f.asked = append(f.asked, n)
	return f.id, f.venue, f.found, f.err
}

func pinRow(n chain.Network) sqlc.GetProposalRow {
	return sqlc.GetProposalRow{ID: testkit.NewIDs(5367).NewV7(), Network: string(n)}
}

func TestUnquotedPin_listsAnUnlistedPinnedChainWithNoRouteOnItsVenue(t *testing.T) {
	t.Parallel()
	for n, venue := range map[chain.Network]multichain.Venue{
		chain.NetworkSolana: multichain.VenueJupiter, chain.NetworkBase: multichain.VenueUniswap,
	} {
		c := NewProposalComparisons(nil, nil, &fakePinListings{})
		got, err := c.unquotedPin(t.Context(), pinRow(n), marketfake.AAPLx(), &multichain.Comparison{})
		want := multichain.UnavailableVenue{Network: n, Venue: venue, Reason: multichain.UnavailableNoRoute}
		if err != nil || len(got.Ranked) != 0 || !got.Winner.IsZero() || len(got.Unavailable) != 1 ||
			got.Unavailable[0] != want {
			t.Errorf("%s: unquotedPin = %+v, %v, want only %+v unavailable", n, got, err, want)
		}
	}
}

func TestUnquotedPin_namesThePinnedChainsListingWhenNoComparisonListsIt(t *testing.T) {
	t.Parallel()
	listing := multichain.ListingIDFrom(testkit.NewIDs(5368).NewV7())
	listings := &fakePinListings{id: listing, venue: multichain.VenueUniswap, found: true}
	c := NewProposalComparisons(nil, nil, listings)
	got, err := c.unquotedPin(t.Context(), pinRow(chain.NetworkBase), marketfake.AAPLx(), &multichain.Comparison{})
	want := multichain.UnavailableVenue{
		ListingID: listing, Network: chain.NetworkBase, Venue: multichain.VenueUniswap,
		Reason: multichain.UnavailableNoRoute,
	}
	if err != nil || got.Winner != listing || len(got.Unavailable) != 1 || got.Unavailable[0] != want {
		t.Fatalf("unquotedPin = %+v, %v, want only %+v unavailable", got, err, want)
	}
	if len(listings.asked) != 1 || listings.asked[0] != chain.NetworkBase {
		t.Fatalf("looked up listings on %v, want base once", listings.asked)
	}
}

func TestUnquotedPin_aFailedListingLookupFailsTheComparison(t *testing.T) {
	t.Parallel()
	lookup := errs.New(errs.CodeInternal, "test")
	c := NewProposalComparisons(nil, nil, &fakePinListings{err: lookup})
	got, err := c.unquotedPin(t.Context(), pinRow(chain.NetworkBase), marketfake.AAPLx(), &multichain.Comparison{})
	if !errors.Is(err, lookup) || got != nil {
		t.Fatalf("unquotedPin = %+v, %v, want the lookup error", got, err)
	}
}
