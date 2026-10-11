package app_test

import (
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
)

func TestRouterListsAVenueThatOutlivesItsBudgetAsUpstreamUnavailable(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c, l, f, err := route(t, map[string]recordedQuote{
			aaplxSolana: recordedApple()[aaplxSolana], aaplcBase: {block: true},
		}, testRules(), func(f *routeFixture) { f.routerClock = clock.Real{} })
		if err != nil || c.Winner != l[aaplxSolana].ID {
			t.Fatalf("comparison = %+v, %v, want the Solana quote", c, err)
		}
		want := []multichain.UnavailableVenue{{
			ListingID: l[aaplcBase].ID, Network: chain.NetworkBase, Venue: multichain.VenueUniswap,
			Reason: multichain.UnavailableStale,
		}}
		if !slices.Equal(c.Unavailable, want) {
			t.Fatalf("unavailable = %+v, want %+v", c.Unavailable, want)
		}
		got := f.logs.Lines("market.router.venue_dropped", "listing_id", "reason", "duration_ms")
		if line := l[aaplcBase].ID.String() + " upstream_unavailable 5000"; !slices.Equal(got, []string{line}) {
			t.Fatalf("venue_dropped = %v, want %q", got, line)
		}
		errText := f.logs.Lines("market.router.venue_dropped", "err")
		if len(errText) != 1 || !strings.Contains(errText[0], "context deadline exceeded") {
			t.Fatalf("venue_dropped err = %v, want the deadline", errText)
		}
	})
}

func TestRouterBridgesOncePerNetworkForAComparison(t *testing.T) {
	t.Parallel()
	base := recordedApple()[aaplcBase]
	c, l, f, err := route(t, map[string]recordedQuote{
		aaplxSolana: recordedApple()[aaplxSolana], aaplcBase: base, aaplxBase: base,
	}, testRules())
	if err != nil || len(c.Ranked) != 3 {
		t.Fatalf("comparison = %+v, %v, want all three listings ranked", c, err)
	}
	if f.bridges[chain.NetworkBase] != 1 {
		t.Fatalf("bridge estimates for Base = %d, want one shared by both Base listings", f.bridges[chain.NetworkBase])
	}
	bridged := map[multichain.ListingID]uint64{}
	for _, q := range c.Ranked {
		bridged[q.ListingID] = costsOf(q)[multichain.CostBridge]
	}
	if fee := bridged[l[aaplcBase].ID]; fee == 0 || bridged[l[aaplxBase].ID] != fee {
		t.Fatalf("bridge costs = %v, want the same nonzero fee on both Base listings", bridged)
	}
}
