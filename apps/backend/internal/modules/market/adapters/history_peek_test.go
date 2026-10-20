package adapters

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type gateHistory struct {
	gate  chan struct{}
	calls atomic.Int64
	err   error
}

func (g *gateHistory) Configured() bool { return true }

func (g *gateHistory) MarketChart(context.Context, domain.Mint, int) (app.History, error) {
	g.calls.Add(1)
	<-g.gate
	if g.err != nil {
		return app.History{}, g.err
	}
	return app.History{Prices: []app.Sample{{At: marketWhen(), Price: money.MicrosFromUint64(5_000_000)}}}, nil
}

func peekMint(t *testing.T) domain.Mint {
	t.Helper()
	mint, err := domain.ParseMint("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	if err != nil {
		t.Fatal(err)
	}
	return mint
}

func TestCachedHistory_aPeekAtAColdMintReturnsAtOnceAndFillsTheCacheInTheBackground(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		fake := &gateHistory{gate: make(chan struct{})}
		cached := CacheHistory(fake, NewCache(testkit.NewClock(marketWhen())))
		mint := peekMint(t)
		for range 3 {
			if _, found, err := cached.Peek(t.Context(), mint, 365); found || err != nil {
				t.Fatalf("cold peek = %v, %v", found, err)
			}
		}
		synctest.Wait()
		if fake.calls.Load() != 1 {
			t.Fatalf("upstream calls = %d, want one shared load", fake.calls.Load())
		}
		close(fake.gate)
		synctest.Wait()
		history, found, err := cached.Peek(t.Context(), mint, 365)
		if !found || err != nil || len(history.Prices) != 1 || fake.calls.Load() != 1 {
			t.Fatalf("warm peek = %+v, %v, %v after %d calls", history, found, err, fake.calls.Load())
		}
	})
}

func TestCachedHistory_aWaitingReadJoinsTheLoadAPeekStarted(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		fake := &gateHistory{gate: make(chan struct{})}
		cached := CacheHistory(fake, NewCache(testkit.NewClock(marketWhen())))
		mint := peekMint(t)
		if _, found, err := cached.Peek(t.Context(), mint, 7); found || err != nil {
			t.Fatalf("cold peek = %v, %v", found, err)
		}
		var got app.History
		var group errgroup.Group
		group.Go(func() (err error) {
			got, err = cached.MarketChart(t.Context(), mint, 7)
			return err
		})
		synctest.Wait()
		close(fake.gate)
		err := group.Wait()
		if err != nil || len(got.Prices) != 1 || fake.calls.Load() != 1 {
			t.Fatalf("read = %+v, %v after %d upstream calls", got, err, fake.calls.Load())
		}
	})
}

func TestCachedHistory_aMintCoinGeckoDoesNotKnowIsRememberedForPeeks(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		fake := &gateHistory{gate: make(chan struct{}), err: errs.New(errs.CodeNotFound, "test")}
		cached := CacheHistory(fake, NewCache(testkit.NewClock(marketWhen())))
		mint := peekMint(t)
		if _, found, err := cached.Peek(t.Context(), mint, 7); found || err != nil {
			t.Fatalf("cold peek = %v, %v", found, err)
		}
		close(fake.gate)
		synctest.Wait()
		if _, found, err := cached.Peek(t.Context(), mint, 7); found || errs.CodeOf(err) != errs.CodeNotFound {
			t.Fatalf("peek = %v, %v, want not_found", found, err)
		}
		if fake.calls.Load() != 1 {
			t.Fatalf("upstream calls = %d", fake.calls.Load())
		}
	})
}

func TestCachedHistory_aFailedBackgroundLoadIsNotCachedAndTheNextPeekTriesAgain(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		fake := &gateHistory{gate: make(chan struct{}), err: errs.New(errs.CodeInternal, "test")}
		cached := CacheHistory(fake, NewCache(testkit.NewClock(marketWhen())))
		mint := peekMint(t)
		close(fake.gate)
		for want := int64(1); want <= 2; want++ {
			if _, found, err := cached.Peek(t.Context(), mint, 7); found || err != nil {
				t.Fatalf("peek = %v, %v", found, err)
			}
			synctest.Wait()
			if fake.calls.Load() != want {
				t.Fatalf("upstream calls = %d, want %d", fake.calls.Load(), want)
			}
		}
	})
}
