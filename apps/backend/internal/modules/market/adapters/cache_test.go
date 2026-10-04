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
				_, err := cached.Handle(t.Context(), "AAPLx", "1D")
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
		chart, err := cached.Handle(t.Context(), "AAPLx", "1D")
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
			_, leader = cached.Handle(leaderCtx, "AAPLx", "1D")
			return nil
		})
		synctest.Wait()
		for range 10 {
			group.Go(func() error {
				_, err := cached.Handle(t.Context(), "AAPLx", "1D")
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
		if _, err := cached.Handle(t.Context(), "AAPLx", "1D"); err != nil || fake.calls.Load() != 1 {
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
			_, err := cached.Handle(t.Context(), "AAPLx", "1D")
			return err
		})
		synctest.Wait()
		stored := app.Chart{Empty: true, Points: []app.Point{}}
		store.keep(chartKey("AAPLx", domain.Chart1D), chartBox{v: stored}, shortTTL)
		close(hold)
		if err := group.Wait(); err != nil || fake.calls.Load() != 0 {
			t.Fatalf("calls = %d err %v", fake.calls.Load(), err)
		}
	})
}

func TestChart_expiresOnTheTTLBoundary(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw string
		ttl time.Duration
	}{
		{"1D", shortTTL},
		{"1W", longTTL},
		{"1M", longTTL},
		{"3M", longTTL},
		{"1Y", longTTL},
		{"ALL", longTTL},
	}
	for _, tc := range cases {
		clk := testkit.NewClock(marketWhen())
		fake := &countingChart{}
		cached := CacheChart(fake, NewCache(clk))
		if _, err := cached.Handle(t.Context(), "AAPLx", tc.raw); err != nil {
			t.Fatalf("%s load = %v", tc.raw, err)
		}
		clk.Advance(tc.ttl - time.Nanosecond)
		if _, err := cached.Handle(t.Context(), "AAPLx", tc.raw); err != nil || fake.calls.Load() != 1 {
			t.Fatalf("%s inside = %d %v", tc.raw, fake.calls.Load(), err)
		}
		clk.Advance(time.Nanosecond)
		if _, err := cached.Handle(t.Context(), "AAPLx", tc.raw); err != nil || fake.calls.Load() != 2 {
			t.Fatalf("%s boundary = %d %v", tc.raw, fake.calls.Load(), err)
		}
	}
}

func TestChart_doesNotCacheABadRangeOrAnError(t *testing.T) {
	t.Parallel()
	fake := &countingChart{}
	cached := CacheChart(fake, NewCache(testkit.NewClock(marketWhen())))
	_, err := cached.Handle(t.Context(), "AAPLx", "nope")
	if errs.CodeOf(err) != errs.CodeInvalidInput || fake.calls.Load() != 0 {
		t.Fatalf("bad range = %v calls %d", err, fake.calls.Load())
	}
	brokenFake := &errChart{}
	broken := CacheChart(brokenFake, NewCache(testkit.NewClock(marketWhen())))
	if _, err := broken.Handle(t.Context(), "AAPLx", "1D"); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("first = %v", err)
	}
	_, err = broken.Handle(t.Context(), "AAPLx", "1D")
	if errs.CodeOf(err) != errs.CodeInternal || brokenFake.calls.Load() != 2 {
		t.Fatalf("second = %d %v", brokenFake.calls.Load(), err)
	}
}

func TestAssets_cachesTheFullQuery(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(marketWhen())
	fake := &countingList{}
	h := HTTP{List: CacheList(fake, NewCache(clk))}
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
	h := HTTP{List: CacheList(fake, NewCache(testkit.NewClock(marketWhen())))}
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

func (c *countingChart) Handle(context.Context, string, string) (app.Chart, error) {
	c.calls.Add(1)
	return app.Chart{Empty: true, Points: []app.Point{}}, nil
}

type errChart struct{ calls atomic.Int32 }

func (c *errChart) Handle(context.Context, string, string) (app.Chart, error) {
	c.calls.Add(1)
	return app.Chart{}, errs.New(errs.CodeInternal, "market.test")
}

type gateChart struct {
	calls atomic.Int32
	gate  <-chan struct{}
}

func (g *gateChart) Handle(ctx context.Context, _, _ string) (app.Chart, error) {
	g.calls.Add(1)
	select {
	case <-ctx.Done():
		return app.Chart{}, ctx.Err()
	case <-g.gate:
		return app.Chart{Empty: true, Points: []app.Point{}}, nil
	}
}
