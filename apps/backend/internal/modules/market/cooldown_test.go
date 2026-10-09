package market_test

import (
	"context"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
)

func rateLimited(afterS int) error {
	if afterS == 0 {
		return errs.New(errs.CodeCoinGeckoRateLimited, "test")
	}
	return errs.New(errs.CodeCoinGeckoRateLimited, "test", slog.Int("retry_after_s", afterS))
}

func TestCooldown_aShorterTripNeverShortensALongerOne(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		clk := clock.Real{}
		c := app.NewCooldown(clk)
		c.Trip(t.Context(), "market.backfill", rateLimited(900))
		<-clk.After(time.Minute)
		c.Trip(t.Context(), "market.reconcile", rateLimited(0))
		<-clk.After(2 * time.Minute)
		if !c.Active() {
			t.Fatal("cooldown over after 3m, want the 15m Retry-After kept")
		}
		<-clk.After(12*time.Minute + time.Second)
		if c.Active() {
			t.Fatal("cooldown still on after the 15m Retry-After")
		}
	})
}

func TestCooldown_waitFollowsAnExtensionMadeWhileWaiting(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		clk := clock.Real{}
		c := app.NewCooldown(clk)
		start := clk.Now()
		c.Trip(t.Context(), "market.backfill", rateLimited(0))
		var extend errgroup.Group
		extend.Go(func() error {
			<-clk.After(30 * time.Second)
			c.Trip(t.Context(), "market.reconcile", rateLimited(300))
			return nil
		})
		defer func() { _ = extend.Wait() }()
		if err := c.Wait(t.Context()); err != nil {
			t.Fatal(err)
		}
		if got := clk.Now().Sub(start); got != 330*time.Second {
			t.Fatalf("waited %v, want 330s: the extension made mid-wait", got)
		}
	})
}

func TestCooldown_waitStopsWhenTheContextEnds(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c := app.NewCooldown(clock.Real{})
		c.Trip(t.Context(), "market.backfill", rateLimited(0))
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		if err := c.Wait(ctx); err == nil {
			t.Fatal("Wait returned nil inside the cooldown, want the context error")
		}
	})
}
