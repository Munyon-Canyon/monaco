package app_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type countingQuoter struct {
	next  multichain.Quoter
	calls atomic.Int64
	gate  chan struct{}
}

func (q *countingQuoter) Quote(
	ctx context.Context, l multichain.Listing, side multichain.Side, amount money.Micros,
) (multichain.Quote, error) {
	q.calls.Add(1)
	if q.gate != nil {
		<-q.gate
	}
	return q.next.Quote(ctx, l, side, amount)
}

type prefetchWorld struct {
	f      *routeFixture
	byName map[string]multichain.Listing
	quoter *countingQuoter
	best   *app.BestChains
	logs   *testkit.Logs
}

func newPrefetchWorld(t *testing.T, catalog *marketfake.CatalogFake) *prefetchWorld {
	t.Helper()
	g := testkit.NewIDs(5364)
	byName := appleListings(t, g)
	f := &routeFixture{
		clk:    testkit.NewClock(clock.Real{}.Now().Truncate(time.Second)),
		quotes: map[multichain.ListingID]recordedQuote{},
		inputs: map[multichain.ListingID]money.Micros{},
	}
	for name, q := range recordedApple() {
		if name == aaplxSolana || name == aaplcBase {
			f.listings = append(f.listings, byName[name])
			f.quotes[byName[name].ID] = q
		}
	}
	quoter := &countingQuoter{next: fixtureQuoter{f: f}}
	router := app.NewRouter(f, app.Venues{multichain.VenueJupiter: quoter, multichain.VenueUniswap: quoter}, f, f,
		f.clk, testRules())
	w := &prefetchWorld{f: f, byName: byName, quoter: quoter, logs: &testkit.Logs{}}
	w.best = app.NewBestChains(catalog, router, f.clk)
	return w
}

func (w *prefetchWorld) context(t *testing.T) context.Context {
	t.Helper()
	return observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, w.logs))
}

func (w *prefetchWorld) get(t *testing.T) app.BestChain {
	t.Helper()
	got, err := w.best.Get(w.context(t), "AAPLx")
	if err != nil {
		t.Fatalf("Get(AAPLx) err = %v", err)
	}
	return got
}

func (w *prefetchWorld) expectQuotes(t *testing.T, want int64, when string) {
	t.Helper()
	if got := w.quoter.calls.Load(); got != want {
		t.Fatalf("%s: quoter calls = %d, want %d", when, got, want)
	}
}

func TestBestChainsQuotesADollarOnEveryChainAndNamesTheWinner(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		w := newPrefetchWorld(t, marketfake.NewCatalog(marketfake.AAPLx()))
		start := w.f.clk.Now()
		got := w.get(t)
		w.expectQuotes(t, 2, "first read")
		sol, base := w.byName[aaplxSolana].ID, w.byName[aaplcBase].ID
		if in := w.f.inputs[sol]; in.Uint64() != 1_000_000 {
			t.Fatalf("Solana quoted %s micros, want the $1 probe", in)
		}
		expectSolanaAheadOfBase(t, got, start, sol, base)
		lines := w.logs.Lines("market.best_chain.refreshed", "symbol", "network", "venue", "duration_ms")
		if len(lines) != 1 || lines[0] != "AAPLx solana jupiter 0" {
			t.Fatalf("refreshed lines = %q", lines)
		}
	})
}

func expectSolanaAheadOfBase(t *testing.T, got app.BestChain, start time.Time, sol, base multichain.ListingID) {
	t.Helper()
	if got.Symbol != "AAPLx" || got.Amount.Uint64() != 1_000_000 || !got.QuotedAt.Equal(start) {
		t.Fatalf("best chain = %s %s at %v, want AAPLx for $1 at %v", got.Symbol, got.Amount, got.QuotedAt, start)
	}
	winner, c := got.Winner(), got.Comparison
	if winner.ListingID != sol || winner.Network != chain.NetworkSolana || c.Winner != sol || len(c.Ranked) != 2 ||
		c.Ranked[1].ListingID != base {
		t.Fatalf("comparison = %+v, want Solana ahead of Base, whose gas eats 7.6%% of $1", c)
	}
	if winner.NetShares != 297_076_362 {
		t.Fatalf("net shares per $1 = %d", winner.NetShares)
	}
}

func TestBestChainsServesAFreshAnswerWithoutQuotingAgain(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		w := newPrefetchWorld(t, marketfake.NewCatalog(marketfake.AAPLx()))
		first := w.get(t)
		w.f.clk.Advance(app.BestChainFresh - time.Second)
		if got := w.get(t); !got.QuotedAt.Equal(first.QuotedAt) {
			t.Fatalf("quoted at %v, want the first answer from %v", got.QuotedAt, first.QuotedAt)
		}
		synctest.Wait()
		w.expectQuotes(t, 2, "19 s later")
	})
}

func TestBestChainsServesAStaleAnswerWhileOneRefreshRuns(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		w := newPrefetchWorld(t, marketfake.NewCatalog(marketfake.AAPLx()))
		first := w.get(t)
		w.quoter.gate = make(chan struct{})
		w.f.clk.Advance(app.BestChainFresh)
		for range 3 {
			if got := w.get(t); !got.QuotedAt.Equal(first.QuotedAt) {
				t.Fatalf("quoted at %v, want the stale answer from %v", got.QuotedAt, first.QuotedAt)
			}
		}
		synctest.Wait()
		w.expectQuotes(t, 4, "three stale reads")
		close(w.quoter.gate)
		synctest.Wait()
		if got := w.get(t); !got.QuotedAt.Equal(first.QuotedAt.Add(app.BestChainFresh)) {
			t.Fatalf("quoted at %v, want the refreshed answer", got.QuotedAt)
		}
		w.expectQuotes(t, 4, "after the refresh")
	})
}

func TestBestChainsRefreshesAnAnswerPastAMinuteBeforeAnswering(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		w := newPrefetchWorld(t, marketfake.NewCatalog(marketfake.AAPLx()))
		first := w.get(t)
		w.f.clk.Advance(app.BestChainServed)
		if got := w.get(t); !got.QuotedAt.Equal(first.QuotedAt.Add(app.BestChainServed)) {
			t.Fatalf("quoted at %v, want a fresh answer", got.QuotedAt)
		}
		w.expectQuotes(t, 4, "a minute later")
	})
}

func TestBestChainsKeepsTheStaleAnswerWhenItsRefreshFails(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		w := newPrefetchWorld(t, marketfake.NewCatalog(marketfake.AAPLx()))
		first := w.get(t)
		w.f.listingsErr = errs.New(errs.CodeJupiterUnavailable, "test")
		w.f.clk.Advance(30 * time.Second)
		for attempt := 1; attempt <= 2; attempt++ {
			if got := w.get(t); !got.QuotedAt.Equal(first.QuotedAt) {
				t.Fatalf("read %d: quoted at %v, want the stale answer from %v", attempt, got.QuotedAt, first.QuotedAt)
			}
			synctest.Wait()
			w.expectFailedRefreshes(t, attempt)
		}
		w.f.clk.Advance(app.BestChainServed - 30*time.Second)
		got, err := w.best.Get(w.context(t), "AAPLx")
		if errs.CodeOf(err) != errs.CodeJupiterUnavailable || !got.QuotedAt.IsZero() {
			t.Fatalf("a minute later: Get = %v, %v, want the refresh error, not the stale answer", got.QuotedAt, err)
		}
		w.expectFailedRefreshes(t, 2)
	})
}

func (w *prefetchWorld) expectFailedRefreshes(t *testing.T, want int) {
	t.Helper()
	lines := w.logs.Lines("market.best_chain.refresh_failed", "symbol", "code")
	if len(lines) != want {
		t.Fatalf("refresh_failed lines = %q, want %d", lines, want)
	}
	for _, line := range lines {
		if line != "AAPLx jupiter_unavailable" {
			t.Fatalf("refresh_failed line = %q", line)
		}
	}
}

func TestBestChainsAnswersWhatItCannotQuote(t *testing.T) {
	t.Parallel()
	bad := marketfake.AAPLx()
	bad.Symbol, bad.ID = "BADx", market.AssetID{}
	for name, tc := range map[string]struct {
		symbol string
		down   error
		want   errs.Code
	}{
		"unknown":    {symbol: "NOPEx", want: errs.CodeAssetNotFound},
		"untradable": {symbol: "JPSTx", want: errs.CodeAssetUntradable},
		"bad id":     {symbol: "BADx", want: errs.CodeDecodeFailed},
		"router":     {symbol: "AAPLx", down: errs.New(errs.CodeJupiterUnavailable, "test"), want: errs.CodeJupiterUnavailable},
	} {
		w := newPrefetchWorld(t, marketfake.NewCatalog(marketfake.AAPLx(), marketfake.JPSTx(), bad))
		w.f.listingsErr = tc.down
		if _, err := w.best.Get(t.Context(), tc.symbol); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: Get err = %v, want %s", name, err, tc.want)
		}
		if tc.down == nil {
			w.expectQuotes(t, 0, name)
		}
	}
}

func TestBestChainsStopsWaitingWhenTheCallerGoes(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		w := newPrefetchWorld(t, marketfake.NewCatalog(marketfake.AAPLx()))
		w.quoter.gate = make(chan struct{})
		ctx, cancel := context.WithCancel(w.context(t))
		var abandoned error
		var group errgroup.Group
		group.Go(func() error {
			_, abandoned = w.best.Get(ctx, "AAPLx")
			return nil
		})
		synctest.Wait()
		cancel()
		_ = group.Wait()
		if !errors.Is(abandoned, context.Canceled) {
			t.Fatalf("Get err = %v, want canceled", abandoned)
		}
		close(w.quoter.gate)
		synctest.Wait()
		w.get(t)
		w.expectQuotes(t, 2, "the abandoned refresh still lands")
	})
}
