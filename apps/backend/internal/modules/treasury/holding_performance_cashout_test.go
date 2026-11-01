package treasury_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestHoldingPerformance_theChartCountsACashOutSellSoItMatchesUnrealized(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	c := testkit.NewCabal(t, f.pool)
	base := clock.Real{}.Now().UTC().Truncate(time.Second).Add(-24 * time.Hour)
	const share = 100_000_000
	h := adapters.Activity{Hints: &hints{}}
	buyID, saleID := f.ids.NewV7(), f.ids.NewV7()
	bought := buy(c.ID, buyID, 2_000_000_000, 10*share, 0)
	bought.Source = events.TradeSource{Kind: "proposal", ID: buyID}
	sold := events.TradeConfirmed{
		V: 1, SwapID: saleID, CabalID: c.ID.UUID(), Source: events.TradeSource{Kind: "cashout", ID: f.ids.NewV7()},
		Action: "sell", Symbol: "AAPLx", InMint: chain.SolanaAddress(aapl), InAmount: 5 * share, OutMint: usdcMint,
		OutAmount: 1_000_000_000, USDCMicros: money.MicrosFromUint64(1_000_000_000), TxSignature: swapSig,
		ConfirmedAt: base.Add(time.Hour),
	}
	if err := f.do(func(ctx context.Context, tx db.Tx) error {
		if err := h.Confirmed(ctx, tx, bought, base); err != nil {
			return err
		}
		return h.Confirmed(ctx, tx, sold, base.Add(time.Hour))
	}); err != nil {
		t.Fatal(err)
	}
	prices := func(context.Context, string, string) ([]app.PricePoint, error) {
		return []app.PricePoint{{At: base.Add(2 * time.Hour), PriceMicros: money.MicrosFromUint64(220_000_000)}}, nil
	}
	reads := app.NewHoldingPerformanceReads(f.pool, member(true), perfNames{}, perfListings, prices, clock.Real{})
	got, err := reads.Handle(
		t.Context(),
		app.HoldingPerformanceQuery{CabalID: c.ID, Caller: c.Creator.ID, Symbol: "AAPLx"},
	)
	if err != nil {
		t.Fatal(err)
	}
	const unrealized = 100_000_000
	series := got.Series["1W"]
	if len(series) == 0 || series[len(series)-1].PnLMicros.Int64() != unrealized {
		t.Fatalf("series = %+v, want the last point at unrealized %d", series, unrealized)
	}
	if got.RealizedPnLMicros.Int64() != 0 {
		t.Fatalf("realized = %d, want 0 for a sale at cost", got.RealizedPnLMicros.Int64())
	}
	seedAAPLx(t, f.pool)
	d := module.Deps{Pool: f.pool}
	feed := app.NewActivityReads(f.pool, cabal.New(d).Queries(), identity.New(d).Queries(),
		treasury.CatalogNames{Catalog: market.New(d).Catalog()})
	page, err := feed.List(t.Context(), app.ListActivity{CabalID: c.ID, Caller: c.Creator.ID})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("feed = %+v, %v, want only the buy: the cash-out sale stays out of the activity feed", page.Items, err)
	}
}

type stepNames struct {
	step time.Time
	next int64
}

func (n stepNames) AssetNames(context.Context) (map[domain.Asset]app.AssetName, error) {
	return map[domain.Asset]app.AssetName{perfMint: {
		Symbol: "AAPLx", Decimals: 8,
		MultiplierAt: func(at time.Time) (int64, int64) {
			if at.Before(n.step) {
				return 1, 1
			}
			return n.next, 1
		},
	}}, nil
}

func pricesAt(points ...app.PricePoint) app.PositionPrices {
	return func(context.Context, string, string) ([]app.PricePoint, error) { return points, nil }
}

func TestHoldingPerformance_aMultiplierStepLeavesEarlierPointsWhereTheyWere(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	base := clock.Real{}.Now().UTC().Truncate(time.Second).Add(-24 * time.Hour)
	step := base.Add(2 * time.Hour)
	const share = 100_000_000
	seedPerfTrade(t, pool, c.ID, perfMint, "buy", "confirmed", 2_000_000_000, 10*share, base)
	prices := pricesAt(
		app.PricePoint{At: step.Add(-time.Hour), PriceMicros: money.MicrosFromUint64(220_000_000)},
		app.PricePoint{At: step.Add(time.Hour), PriceMicros: money.MicrosFromUint64(110_000_000)},
	)
	reads := app.NewHoldingPerformanceReads(pool, member(true), stepNames{step: step, next: 2}, perfListings, prices,
		clock.Real{})
	got, err := reads.Handle(t.Context(),
		app.HoldingPerformanceQuery{CabalID: c.ID, Caller: c.Creator.ID, Symbol: "AAPLx"})
	if err != nil {
		t.Fatal(err)
	}
	series := got.Series["1W"]
	if len(series) != 3 {
		t.Fatalf("series = %+v, want the two priced points and the closing point", series)
	}
	for i, point := range series {
		if point.PnLMicros.Int64() != 200_000_000 {
			t.Fatalf("point %d = %d, want 200000000 before and after the 2:1 step", i, point.PnLMicros.Int64())
		}
	}
}

func TestHoldingPerformance_failsWhenAMultiplierCannotBeRead(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	base := clock.Real{}.Now().UTC().Truncate(time.Second).Add(-24 * time.Hour)
	seedPerfTrade(t, pool, c.ID, perfMint, "buy", "confirmed", 100, 100, base)
	query := app.HoldingPerformanceQuery{CabalID: c.ID, Caller: c.Creator.ID, Symbol: "AAPLx"}
	one := money.MicrosFromUint64(1)
	cases := []struct {
		name   string
		prices app.PositionPrices
		later  bool
	}{
		{name: "a priced point", prices: pricesAt(app.PricePoint{At: base.Add(2 * time.Hour), PriceMicros: one})},
		{name: "the closing point", prices: pricesAt(app.PricePoint{At: base.Add(-time.Hour), PriceMicros: one})},
		{name: "a later trade", later: true},
	}
	for _, tc := range cases {
		if tc.later {
			seedPerfTrade(t, pool, c.ID, perfMint, "buy", "confirmed", 100, 100, base.Add(2*time.Hour))
		}
		reads := app.NewHoldingPerformanceReads(pool, member(true), stepNames{step: base.Add(time.Hour), next: 0},
			perfListings, tc.prices, clock.Real{})
		if _, err := reads.Handle(t.Context(), query); errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("%s with an unreadable multiplier = %v, want internal", tc.name, err)
		}
	}
}
