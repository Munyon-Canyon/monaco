package market_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestAcceptNewest(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now()
	cases := map[string]struct {
		micros []uint64
		want   int
	}{
		"no samples":                  {nil, -1},
		"a lone sample":               {[]uint64{100_000_000}, 0},
		"steady":                      {[]uint64{100_000_000, 101_000_000, 99_000_000}, 0},
		"exactly a fifth":             {[]uint64{120_000_000, 100_000_000}, 0},
		"a spike is held":             {[]uint64{125_000_000, 100_000_000, 100_000_000}, -1},
		"a 40 percent drop is held":   {[]uint64{60_000_000, 100_000_000, 100_000_000}, -1},
		"a confirmed jump is current": {[]uint64{60_000_000, 60_000_000, 100_000_000}, 0},
		"just over a fifth":           {[]uint64{120_000_001, 100_000_000}, -1},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			samples := newestFirst(now, c.micros...)
			got, ok := domain.AcceptNewest(samples)
			if c.want < 0 {
				if ok {
					t.Fatalf("AcceptNewest(%v) = %v, true, want the newest held back", samples, got)
				}
				return
			}
			if !ok || got != samples[c.want] {
				t.Fatalf("AcceptNewest(%v) = %v, %v, want %v", samples, got, ok, samples[c.want])
			}
		})
	}
}

func TestAcceptNewest_neverLooksPastTheNewest(t *testing.T) {
	t.Parallel()
	samples := newestFirst(clock.Real{}.Now(), 60_000_000, 100_000_000, 100_000_000)
	older, ok := domain.Accept(samples)
	if !ok || older != samples[1] {
		t.Fatalf("Accept(%v) = %v, %v, want the older sample the leaderboards keep using", samples, older, ok)
	}
	if got, ok := domain.AcceptNewest(samples); ok {
		t.Fatalf("AcceptNewest(%v) = %v, true, want no price while the drop is held", samples, got)
	}
}

func TestCurrentPricesAsOf_pricesOnlyAssetsWhoseNewestSampleIsAccepted(t *testing.T) {
	t.Parallel()
	r := newPriceRig(t)
	current := market.New(module.Deps{Pool: r.pool, Clock: r.clock}).Prices()
	aapl, tsla, jpst := marketfake.AAPLx(), marketfake.TSLAx(), marketfake.JPSTx()
	r.sample(t, aapl, 200_000_000, r.bucket.Add(-4*time.Minute))
	r.sample(t, aapl, 200_000_000, r.bucket.Add(-2*time.Minute))
	r.sample(t, aapl, 120_000_000, r.bucket)
	r.sample(t, tsla, 300_000_000, r.bucket.Add(-2*time.Minute))
	r.sample(t, tsla, 305_000_000, r.bucket)
	asked := []market.AssetID{aapl.ID, tsla.ID, jpst.ID}

	got, err := current.CurrentPricesAsOf(t.Context(), asked, r.bucket)
	want := map[market.AssetID]market.Price{tsla.ID: {Micros: usd(305_000_000), ObservedAt: r.bucket}}
	if err != nil || !samePrices(got, want) {
		t.Fatalf("CurrentPricesAsOf with AAPLx dropping 40%% = %v, %v, want only TSLAx %v", got, err, want)
	}
	accepted, err := current.PricesAsOf(t.Context(), asked, r.bucket)
	if err != nil || accepted[aapl.ID].Micros != usd(200_000_000) {
		t.Fatalf("PricesAsOf = %v, %v, want leaderboards to keep the older AAPLx sample", accepted, err)
	}

	r.sample(t, aapl, 120_000_000, r.bucket.Add(2*time.Minute))
	got, err = current.CurrentPricesAsOf(t.Context(), asked, r.bucket.Add(2*time.Minute))
	if err != nil || got[aapl.ID].Micros != usd(120_000_000) || len(got) != 2 {
		t.Fatalf("CurrentPricesAsOf once the drop is confirmed = %v, %v, want AAPLx at 120", got, err)
	}
	if got, err := current.CurrentPricesAsOf(t.Context(), nil, r.bucket); err != nil || len(got) != 0 {
		t.Fatalf("CurrentPricesAsOf(no assets) = %v, %v, want nothing", got, err)
	}
}

func TestCurrentPricesAsOf_aDatabaseErrorFailsTheRead(t *testing.T) {
	t.Parallel()
	r := newPriceRig(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	current := market.New(module.Deps{Pool: r.pool, Clock: r.clock}).Prices()
	if _, err := current.CurrentPricesAsOf(ctx, []market.AssetID{marketfake.AAPLx().ID}, r.bucket); err == nil {
		t.Fatal("CurrentPricesAsOf on a canceled context = nil error")
	}
}
