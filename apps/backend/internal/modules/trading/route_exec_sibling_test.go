package trading_test

import (
	"math/big"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	mcfakes "github.com/monaco/monaco/apps/backend/internal/testkit/fakes/multichain"
)

func (e *routeEnv) withSibling(sibling mcfakes.Reply[multichain.Quote], listed bool) multichain.Listing {
	e.listing.SharesPerToken = big.NewRat(1, 1)
	e.listings.listings[e.listing.ID] = e.listing
	solana := multichain.Listing{
		ID: multichain.ListingIDFrom(e.ids.NewV7()), Symbol: "AAPLx", Network: chain.NetworkSolana, Decimals: 8,
		SharesPerToken: big.NewRat(1, 1), Venue: multichain.VenueJupiter, Enabled: true,
	}
	if listed {
		e.listings.listings[solana.ID] = solana
	}
	e.route.Ranked = append(e.route.Ranked, multichain.Quote{ListingID: solana.ID, Network: chain.NetworkSolana})
	e.quotes = mcfakes.New().Quoter
	e.requote()
	e.quotes.Quotes.Push(sibling)
	return solana
}

func siblingOut(units int64) mcfakes.Reply[multichain.Quote] {
	return mcfakes.Reply[multichain.Quote]{Value: multichain.Quote{GrossOutput: big.NewInt(units)}}
}

func TestRouteExec_aRequoteWithin300BpsOfTheSolanaSiblingBuys(t *testing.T) {
	t.Parallel()
	for name, units := range map[string]int64{
		"sibling better":  requotedOut * 10_300 / 10_000,
		"same":            requotedOut,
		"sibling cheaper": requotedOut * 9_750 / 10_000,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			e.withSibling(siblingOut(units), true)
			e.privy.script(multichain.ActionSucceeded)
			e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}

			e.mustBuy(t, e.request(), domain.StatusConfirmed)
		})
	}
}

func TestRouteExec_aRequoteBeyond300BpsOfTheSolanaSiblingFailsPriceMoved(t *testing.T) {
	t.Parallel()
	for name, units := range map[string]int64{
		"evm worse":  requotedOut * 10_400 / 10_000,
		"evm better": requotedOut * 9_600 / 10_000,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			e.withSibling(siblingOut(units), true)
			e.privy.script(multichain.ActionSucceeded)

			view := e.mustBuy(t, e.request(), domain.StatusFailed)

			if code := e.failure(
				t,
				view.ID.UUID(),
			); code != string(domain.FailurePriceMoved) ||
				len(e.evm.keys()) != 0 {
				t.Fatalf("failure %s after sends %v, want price_moved and no swap", code, e.evm.keys())
			}
			wantSteps(t, e.steps(t, view.ID.UUID()), bridged()...)
		})
	}
}

func TestRouteExec_aSiblingThatCannotBeComparedDoesNotStopTheBuy(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		reply  mcfakes.Reply[multichain.Quote]
		listed bool
	}{
		"no route":       {mcfakes.Reply[multichain.Quote]{Code: errs.CodeNoRoute}, true},
		"empty quote":    {mcfakes.Reply[multichain.Quote]{}, true},
		"unlisted":       {siblingOut(requotedOut), false},
		"not quotable":   {mcfakes.Reply[multichain.Quote]{Code: errs.CodeAssetUntradable}, true},
		"no shares unit": {siblingOut(0), true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			e.withSibling(tc.reply, tc.listed)
			e.privy.script(multichain.ActionSucceeded)
			e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}

			e.mustBuy(t, e.request(), domain.StatusConfirmed)
		})
	}
}

func TestRouteExec_aSiblingOutageLeavesTheBridgedSwapForTheResume(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.withSibling(mcfakes.Reply[multichain.Quote]{Code: errs.CodeUpstreamUnavailable}, true)
	e.privy.script(multichain.ActionSucceeded)

	view, err := e.buy(t.Context(), e.request())

	if err == nil || view.Status == domain.StatusFailed {
		t.Fatalf("Buy = %+v, %v, want the retryable outage returned and the swap left alive", view, err)
	}
}

func TestRouteExec_aSiblingGapThatOverflowsIsRefused(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	solana := e.withSibling(siblingOut(1), true)
	solana.Decimals, solana.SharesPerToken = 0, big.NewRat(1, 1_000_000_000)
	e.listings.listings[solana.ID] = solana
	e.listing.Decimals = 0
	e.listings.listings[e.listing.ID] = e.listing
	e.quotes = mcfakes.New().Quoter
	e.quotes.Quotes.Push(siblingOut(4_000_000_000))
	e.quotes.Quotes.Push(siblingOut(1))
	e.privy.script(multichain.ActionSucceeded)

	if _, err := e.buy(t.Context(), e.request()); err == nil {
		t.Fatal("Buy succeeded, want the overflowing share gap refused")
	}
}

func TestRouteExec_aSiblingQuoteTooLargeToCountAsSharesDoesNotStopTheBuy(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	solana := e.withSibling(siblingOut(100_000_000_000), true)
	solana.Decimals = 0
	e.listings.listings[solana.ID] = solana
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}

	e.mustBuy(t, e.request(), domain.StatusConfirmed)
}
