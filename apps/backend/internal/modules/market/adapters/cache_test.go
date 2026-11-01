package adapters

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/marketapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestChart_CacheSingleFlight(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		gate := make(chan struct{})
		fake := &gateChart{gate: gate}
		cached := CacheChart(fake, NewCache(testkit.NewClock(marketWhen())))
		var group errgroup.Group
		for range 50 {
			group.Go(func() error {
				_, err := cached.Handle(t.Context(), "AAPLx", app.ChartRequest{Range: "1D"})
				return err
			})
		}
		synctest.Wait()
		if fake.calls.Load() != 1 {
			t.Fatalf("misses = %d", fake.calls.Load())
		}
		close(gate)
		if err := group.Wait(); err != nil {
			t.Fatal(err)
		}
		chart, err := cached.Handle(t.Context(), "AAPLx", app.ChartRequest{Range: "1D"})
		if err != nil || !chart.Empty || fake.calls.Load() != 1 {
			t.Fatalf("cached = %+v calls %d err %v", chart, fake.calls.Load(), err)
		}
	})
}

func TestChart_aCanceledLeaderDoesNotFailTheRest(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		gate := make(chan struct{})
		fake := &gateChart{gate: gate}
		cached := CacheChart(fake, NewCache(testkit.NewClock(marketWhen())))
		leaderCtx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var leader error
		var group errgroup.Group
		group.Go(func() error {
			_, leader = cached.Handle(leaderCtx, "AAPLx", app.ChartRequest{Range: "1D"})
			return nil
		})
		synctest.Wait()
		for range 10 {
			group.Go(func() error {
				_, err := cached.Handle(t.Context(), "AAPLx", app.ChartRequest{Range: "1D"})
				return err
			})
		}
		synctest.Wait()
		cancel()
		synctest.Wait()
		close(gate)
		if err := group.Wait(); err != nil {
			t.Fatal(err)
		}
		if !errors.Is(leader, context.Canceled) || fake.calls.Load() != 1 {
			t.Fatalf("leader = %v calls %d", leader, fake.calls.Load())
		}
		if _, err := cached.Handle(
			t.Context(),
			"AAPLx",
			app.ChartRequest{Range: "1D"},
		); err != nil ||
			fake.calls.Load() != 1 {
			t.Fatalf("stored = %d %v", fake.calls.Load(), err)
		}
	})
}

func TestChart_aLateMissUsesWhatTheFirstLoadStored(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		hold := make(chan struct{})
		clk := &stepClock{Clock: testkit.NewClock(marketWhen()), hold: hold}
		fake := &countingChart{}
		store := NewCache(clk)
		cached := CacheChart(fake, store)
		var group errgroup.Group
		group.Go(func() error {
			_, err := cached.Handle(t.Context(), "AAPLx", app.ChartRequest{Range: "1D"})
			return err
		})
		synctest.Wait()
		stored := app.Chart{Empty: true, Points: []app.Point{}}
		store.keep(chartKey("AAPLx", domain.Chart1D, app.ChartRequest{Range: "1D"}), chartBox{v: stored}, shortTTL)
		close(hold)
		if err := group.Wait(); err != nil || fake.calls.Load() != 0 {
			t.Fatalf("calls = %d err %v", fake.calls.Load(), err)
		}
	})
}

func TestChart_expiresOnTheTTLBoundary(t *testing.T) {
	t.Parallel()
	before := marketWhen().Add(-time.Hour)
	cases := []struct {
		req app.ChartRequest
		ttl time.Duration
	}{
		{app.ChartRequest{Range: "1D"}, shortTTL},
		{app.ChartRequest{Range: "1W"}, shortTTL},
		{app.ChartRequest{Range: "1Y"}, shortTTL},
		{app.ChartRequest{Range: "ALL"}, shortTTL},
		{app.ChartRequest{Range: "1D", Before: &before}, longTTL},
		{app.ChartRequest{Range: "1W", Before: &before}, longTTL},
	}
	for _, tc := range cases {
		clk := testkit.NewClock(marketWhen())
		fake := &countingChart{}
		cached := CacheChart(fake, NewCache(clk))
		if _, err := cached.Handle(t.Context(), "AAPLx", tc.req); err != nil {
			t.Fatalf("%+v load = %v", tc.req, err)
		}
		clk.Advance(tc.ttl - time.Nanosecond)
		if _, err := cached.Handle(t.Context(), "AAPLx", tc.req); err != nil || fake.calls.Load() != 1 {
			t.Fatalf("%+v inside = %d %v", tc.req, fake.calls.Load(), err)
		}
		clk.Advance(time.Nanosecond)
		if _, err := cached.Handle(t.Context(), "AAPLx", tc.req); err != nil || fake.calls.Load() != 2 {
			t.Fatalf("%+v boundary = %d %v", tc.req, fake.calls.Load(), err)
		}
	}
}

func TestChart_cachesEachIntervalLimitAndPageSeparately(t *testing.T) {
	t.Parallel()
	before := marketWhen().Add(-time.Hour)
	fake := &countingChart{}
	cached := CacheChart(fake, NewCache(testkit.NewClock(marketWhen())))
	reqs := []app.ChartRequest{
		{Range: "1W"},
		{Range: "1W", Interval: "15m"},
		{Range: "1W", Interval: "1h"},
		{Range: "1Y", Interval: "1w"},
		{Range: "1Y", Interval: "1d"},
		{Range: "1W", Interval: "15m", Limit: 5},
		{Range: "1W", Interval: "15m", Before: &before},
	}
	for round := range 2 {
		for _, req := range reqs {
			if _, err := cached.Handle(t.Context(), "AAPLx", req); err != nil {
				t.Fatalf("round %d %+v = %v", round, req, err)
			}
		}
	}
	if int(fake.calls.Load()) != len(reqs) {
		t.Fatalf("loads = %d, want one per distinct request (%d)", fake.calls.Load(), len(reqs))
	}
}

func TestChart_doesNotCacheABadRangeOrAnError(t *testing.T) {
	t.Parallel()
	fake := &countingChart{}
	cached := CacheChart(fake, NewCache(testkit.NewClock(marketWhen())))
	_, err := cached.Handle(t.Context(), "AAPLx", app.ChartRequest{Range: "nope"})
	if errs.CodeOf(err) != errs.CodeInvalidInput || fake.calls.Load() != 0 {
		t.Fatalf("bad range = %v calls %d", err, fake.calls.Load())
	}
	brokenFake := &errChart{}
	broken := CacheChart(brokenFake, NewCache(testkit.NewClock(marketWhen())))
	if _, err := broken.Handle(
		t.Context(),
		"AAPLx",
		app.ChartRequest{Range: "1D"},
	); errs.CodeOf(
		err,
	) != errs.CodeInternal {
		t.Fatalf("first = %v", err)
	}
	_, err = broken.Handle(t.Context(), "AAPLx", app.ChartRequest{Range: "1D"})
	if errs.CodeOf(err) != errs.CodeInternal || brokenFake.calls.Load() != 2 {
		t.Fatalf("second = %d %v", brokenFake.calls.Load(), err)
	}
}

func TestAssets_cachesTheFullQuery(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(marketWhen())
	fake := &countingList{}
	h := HTTP{List: CacheList(fake, NewCache(clk)), Clock: clk}
	user := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: testkit.NewIDs(1).NewV7().String()})
	if _, err := h.GetAssets(user, api.GetAssetsRequestObject{}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.GetAssets(user, api.GetAssetsRequestObject{}); err != nil || fake.calls.Load() != 1 {
		t.Fatalf("repeat = %d %v", fake.calls.Load(), err)
	}
	q := "apple"
	named := api.GetAssetsRequestObject{Params: api.GetAssetsParams{Q: &q}}
	if _, err := h.GetAssets(user, named); err != nil || fake.calls.Load() != 2 {
		t.Fatalf("named = %d %v", fake.calls.Load(), err)
	}
	fake.err = errs.New(errs.CodeInternal, "market.test")
	badQ := "boom"
	bad := api.GetAssetsRequestObject{Params: api.GetAssetsParams{Q: &badQ}}
	if _, err := h.GetAssets(user, bad); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("first error = %v", err)
	}
	if _, err := h.GetAssets(user, bad); errs.CodeOf(err) != errs.CodeInternal || fake.calls.Load() != 4 {
		t.Fatalf("second error = %d %v", fake.calls.Load(), err)
	}
	fake.err = nil
	clk.Advance(shortTTL)
	if _, err := h.GetAssets(user, api.GetAssetsRequestObject{}); err != nil || fake.calls.Load() != 5 {
		t.Fatalf("expired = %d %v", fake.calls.Load(), err)
	}
}

func TestAssets_aCanceledReadDoesNotCallNext(t *testing.T) {
	t.Parallel()
	fake := &countingList{}
	h := HTTP{List: CacheList(fake, NewCache(testkit.NewClock(marketWhen()))), Clock: testkit.NewClock(marketWhen())}
	user := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: testkit.NewIDs(1).NewV7().String()})
	ctx, cancel := context.WithCancel(user)
	cancel()
	_, err := h.GetAssets(ctx, api.GetAssetsRequestObject{})
	if !errors.Is(err, context.Canceled) || fake.calls.Load() != 0 {
		t.Fatalf("canceled = %d %v", fake.calls.Load(), err)
	}
}

func TestCache_doesNotStorePastTheLimit(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(marketWhen())
	cache := NewCache(clk)
	for i := range cacheLimit {
		cache.keep(fmt.Sprintf("key-%d", i), pageBox{}, shortTTL)
	}
	req := app.ListRequest{Q: "past-limit"}
	key := listKey(req)
	cache.keep(key, pageBox{}, shortTTL)
	if len(cache.items) != cacheLimit {
		t.Fatalf("items = %d", len(cache.items))
	}
	if _, ok := cache.hit(key); ok {
		t.Fatal("past-limit was stored")
	}
	fake := &countingList{}
	if _, err := CacheList(fake, cache).Handle(t.Context(), req); err != nil || fake.calls.Load() != 1 {
		t.Fatalf("past-limit served = %d %v", fake.calls.Load(), err)
	}
}

func marketWhen() time.Time { return time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC) }

type stepClock struct {
	*testkit.Clock
	hits atomic.Int32
	hold <-chan struct{}
}

func (s *stepClock) Now() time.Time {
	if s.hits.Add(1) == 2 {
		<-s.hold
	}
	return s.Clock.Now()
}

type countingList struct {
	calls atomic.Int32
	err   error
}

func (c *countingList) Handle(context.Context, app.ListRequest) (app.Page, error) {
	c.calls.Add(1)
	if c.err != nil {
		return app.Page{}, c.err
	}
	return app.Page{}, nil
}

type countingChart struct{ calls atomic.Int32 }

func (c *countingChart) Handle(context.Context, string, app.ChartRequest) (app.Chart, error) {
	c.calls.Add(1)
	return app.Chart{Empty: true, Points: []app.Point{}}, nil
}

type errChart struct{ calls atomic.Int32 }

func (c *errChart) Handle(context.Context, string, app.ChartRequest) (app.Chart, error) {
	c.calls.Add(1)
	return app.Chart{}, errs.New(errs.CodeInternal, "market.test")
}

type gateChart struct {
	calls atomic.Int32
	gate  <-chan struct{}
}

func (g *gateChart) Handle(ctx context.Context, _ string, _ app.ChartRequest) (app.Chart, error) {
	g.calls.Add(1)
	select {
	case <-ctx.Done():
		return app.Chart{}, ctx.Err()
	case <-g.gate:
		return app.Chart{Empty: true, Points: []app.Point{}}, nil
	}
}

type countingHistory struct {
	calls   atomic.Int32
	err     error
	samples []app.Sample
	off     bool
}

func (c *countingHistory) Configured() bool { return !c.off }

func (c *countingHistory) MarketChart(context.Context, domain.Mint, int) (app.History, error) {
	c.calls.Add(1)
	return app.History{Prices: c.samples}, c.err
}

func historyMint(t *testing.T) domain.Mint {
	t.Helper()
	m, err := domain.ParseMint("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestHistory_aDayIsCachedForAnHourAndLongerRangesForSixHours(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(marketWhen())
	fake := &countingHistory{samples: []app.Sample{{At: marketWhen()}}}
	cached := CacheHistory(fake, NewCache(clk))
	mint := historyMint(t)
	ask := func(days int) {
		t.Helper()
		got, err := cached.MarketChart(t.Context(), mint, days)
		if err != nil || len(got.Prices) != 1 {
			t.Fatalf("days %d = %v err %v", days, got, err)
		}
	}
	ask(1)
	ask(7)
	ask(app.HistoryMaxDays)
	ask(1)
	ask(7)
	if fake.calls.Load() != 3 {
		t.Fatalf("calls = %d, want one per (mint, days)", fake.calls.Load())
	}
	clk.Advance(59 * time.Minute)
	ask(1)
	ask(7)
	if fake.calls.Load() != 3 {
		t.Fatalf("calls = %d after 59 minutes, want the day still cached", fake.calls.Load())
	}
	clk.Advance(2 * time.Minute)
	ask(1)
	ask(7)
	if fake.calls.Load() != 4 {
		t.Fatalf("calls = %d after 61 minutes, want only the day refetched", fake.calls.Load())
	}
	clk.Advance(6 * time.Hour)
	ask(7)
	ask(app.HistoryMaxDays)
	if fake.calls.Load() != 6 {
		t.Fatalf("calls = %d after 6 hours, want 7d and max refetched", fake.calls.Load())
	}
}

func TestHistory_aNotFoundIsRememberedForADay(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(marketWhen())
	fake := &countingHistory{err: errs.New(errs.CodeNotFound, "coingecko.test")}
	cached := CacheHistory(fake, NewCache(clk))
	mint := historyMint(t)
	for range 3 {
		if _, err := cached.MarketChart(t.Context(), mint, 7); errs.CodeOf(err) != errs.CodeNotFound {
			t.Fatalf("err = %v", err)
		}
	}
	clk.Advance(23 * time.Hour)
	if _, err := cached.MarketChart(
		t.Context(),
		mint,
		7,
	); errs.CodeOf(err) != errs.CodeNotFound ||
		fake.calls.Load() != 1 {
		t.Fatalf("err = %v calls %d", err, fake.calls.Load())
	}
	clk.Advance(2 * time.Hour)
	if _, err := cached.MarketChart(
		t.Context(),
		mint,
		7,
	); errs.CodeOf(err) != errs.CodeNotFound ||
		fake.calls.Load() != 2 {
		t.Fatalf("err = %v calls %d", err, fake.calls.Load())
	}
}

func TestHistory_anUpstreamErrorIsNotCachedAndConfiguredPassesThrough(t *testing.T) {
	t.Parallel()
	fake := &countingHistory{err: errs.New(errs.CodeUpstreamUnavailable, "coingecko.test"), off: true}
	cached := CacheHistory(fake, NewCache(testkit.NewClock(marketWhen())))
	if cached.Configured() {
		t.Fatal("Configured = true over an unconfigured history")
	}
	for range 2 {
		if _, err := cached.MarketChart(
			t.Context(),
			historyMint(t),
			1,
		); errs.CodeOf(
			err,
		) != errs.CodeUpstreamUnavailable {
			t.Fatalf("err = %v", err)
		}
	}
	if fake.calls.Load() != 2 {
		t.Fatalf("calls = %d, want a retry after an error", fake.calls.Load())
	}
}

func TestHistory_concurrentMissesMakeOneCall(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		fake := &countingHistory{samples: []app.Sample{{At: marketWhen()}}}
		cached := CacheHistory(fake, NewCache(testkit.NewClock(marketWhen())))
		mint := historyMint(t)
		var group errgroup.Group
		for range 20 {
			group.Go(func() error {
				_, err := cached.MarketChart(t.Context(), mint, 30)
				return err
			})
		}
		if err := group.Wait(); err != nil || fake.calls.Load() != 1 {
			t.Fatalf("err = %v calls %d", err, fake.calls.Load())
		}
	})
}

type fixedLevel struct{ level app.BudgetLevel }

func (f *fixedLevel) Level(context.Context) app.BudgetLevel { return f.level }

func TestHistory_aSoftBudgetServesNoUpstreamLoadAndCachesNothingUntilItOpens(t *testing.T) {
	t.Parallel()
	fake := &countingHistory{samples: []app.Sample{{At: marketWhen()}}}
	level := &fixedLevel{level: app.BudgetSoft}
	cached := CacheHistory(fake, NewCache(testkit.NewClock(marketWhen()))).WithBudget(level)
	mint := historyMint(t)
	for range 2 {
		if _, err := cached.MarketChart(t.Context(), mint, 7); errs.CodeOf(err) != errs.CodeNotFound {
			t.Fatalf("MarketChart under a soft budget = %v, want not_found", err)
		}
	}
	if _, found, err := cached.Peek(t.Context(), mint, 7); found || err != nil {
		t.Fatalf("Peek under a soft budget = %v, %v, want a quiet miss", found, err)
	}
	if fake.calls.Load() != 0 {
		t.Fatalf("upstream calls = %d under a soft budget, want 0", fake.calls.Load())
	}
	level.level = app.BudgetOpen
	if got, err := cached.MarketChart(t.Context(), mint, 7); err != nil || len(got.Prices) != 1 {
		t.Fatalf("MarketChart once the budget is open = %v, %v, want the history", got, err)
	}
	if fake.calls.Load() != 1 {
		t.Fatalf("upstream calls = %d once open, want 1: the soft answer was not cached", fake.calls.Load())
	}
}
