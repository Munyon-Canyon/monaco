package treasury_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func seriesNow() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) }

type fixedNow struct{ clock.Real }

func (fixedNow) Now() time.Time { return seriesNow() }

type priceCalls struct {
	points []app.PricePoint
	err    error
	calls  int
}

func (p *priceCalls) read(context.Context, string, string) ([]app.PricePoint, error) {
	p.calls++
	return p.points, p.err
}

func seriesReads(pool *pgxpool.Pool, prices *priceCalls) *app.HoldingPerformanceReads {
	return app.NewHoldingPerformanceReads(pool, member(true), perfNames{}, perfListings, prices.read, fixedNow{})
}

func pricePoint(at time.Time, micros uint64) app.PricePoint {
	return app.PricePoint{At: at, PriceMicros: money.MicrosFromUint64(micros)}
}

func TestHoldingPerformance_seriesEndsAtTheCurrentPositionPnLForEveryRange(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	first := seriesNow().Add(-48 * time.Hour)
	const share = 100_000_000
	seedPerfTrade(t, pool, c.ID, perfMint, "buy", "confirmed", 200_000_000, 2*share, first)
	seedPerfTrade(t, pool, c.ID, perfMint, "sell", "confirmed", 150_000_000, share, first.Add(2*time.Hour))
	prices := &priceCalls{points: []app.PricePoint{
		pricePoint(first.Add(-time.Hour), 90_000_000),
		pricePoint(first.Add(time.Hour), 110_000_000),
		pricePoint(seriesNow().Add(-time.Hour), 120_000_000),
		pricePoint(seriesNow(), 130_000_000),
	}}
	got, err := seriesReads(pool, prices).Handle(t.Context(),
		app.HoldingPerformanceQuery{CabalID: c.ID, Caller: c.Creator.ID, Symbol: "AAPLx"})
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{20_000_000, 70_000_000, 80_000_000}
	wantAt := []time.Time{first.Add(time.Hour), seriesNow().Add(-time.Hour), seriesNow()}
	for _, name := range app.SeriesRanges() {
		series := got.Series[name]
		if len(series) != len(want) {
			t.Fatalf("%s series = %+v, want %d points", name, series, len(want))
		}
		for i, point := range series {
			if point.PnLMicros.Int64() != want[i] || !point.At.Equal(wantAt[i]) {
				t.Fatalf("%s point %d = %+v, want %d at %s", name, i, point, want[i], wantAt[i])
			}
		}
	}
	if prices.calls != len(app.SeriesRanges()) {
		t.Fatalf("price reads = %d, want one per range", prices.calls)
	}
}

func TestHoldingPerformance_seriesKeepsAtMost48PointsOldestFirstEndingAtNow(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	first := seriesNow().Add(-200 * time.Hour)
	seedPerfTrade(t, pool, c.ID, perfMint, "buy", "confirmed", 100_000_000, 100_000_000, first)
	prices := &priceCalls{}
	for i := range 100 {
		prices.points = append(prices.points, pricePoint(first.Add(time.Duration(i)*time.Hour), 100_000_000))
	}
	got, err := seriesReads(pool, prices).Handle(t.Context(),
		app.HoldingPerformanceQuery{CabalID: c.ID, Caller: c.Creator.ID, Symbol: "AAPLx"})
	if err != nil {
		t.Fatal(err)
	}
	series := got.Series["ALL"]
	if len(series) != 48 || !series[0].At.Equal(first) || !series[47].At.Equal(seriesNow()) {
		t.Fatalf("series = %d points from %s to %s, want 48 from %s to %s", len(series), series[0].At,
			series[len(series)-1].At, first, seriesNow())
	}
	for i := 1; i < len(series); i++ {
		if !series[i].At.After(series[i-1].At) {
			t.Fatalf("point %d at %s is not after %s", i, series[i].At, series[i-1].At)
		}
	}
}

func TestHoldingPerformance_seriesIsEmptyWithoutTradesPricesOrAnyTradeBeforeNow(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	query := app.HoldingPerformanceQuery{CabalID: c.ID, Caller: c.Creator.ID, Symbol: "AAPLx"}
	prices := &priceCalls{points: []app.PricePoint{pricePoint(seriesNow(), 100_000_000)}}
	got, err := seriesReads(pool, prices).Handle(t.Context(), query)
	if err != nil || len(got.Series) != 0 || prices.calls != 0 {
		t.Fatalf("never held = %+v, %v, %d price reads, want no series and no reads", got.Series, err, prices.calls)
	}
	seedPerfTrade(t, pool, c.ID, perfMint, "buy", "confirmed", 100_000_000, 100_000_000, seriesNow().Add(time.Hour))
	got, err = seriesReads(pool, prices).Handle(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range app.SeriesRanges() {
		if series := got.Series[name]; len(series) != 1 || series[0].PnLMicros.Int64() != 0 {
			t.Fatalf("%s before the first trade = %+v, want one zero point", name, series)
		}
	}
	got, err = seriesReads(pool, &priceCalls{}).Handle(t.Context(), query)
	if err != nil || len(got.Series) != 0 {
		t.Fatalf("no prices = %+v, %v, want no series", got.Series, err)
	}
}

func TestHoldingPerformance_seriesFailsWhenPricesCannotBeReadOrASharesRatioIsMissing(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	query := app.HoldingPerformanceQuery{CabalID: c.ID, Caller: c.Creator.ID, Symbol: "AAPLx"}
	seedPerfTrade(t, pool, c.ID, perfMint, "buy", "confirmed", 100_000_000, 100_000_000, seriesNow().Add(-time.Hour))
	down := &priceCalls{err: errs.New(errs.CodeDBUnavailable, "test")}
	if _, err := seriesReads(pool, down).Handle(t.Context(), query); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("price failure = %v, want db_unavailable", err)
	}
	seedPerfTrade(t, pool, c.ID, perfEVM, "buy", "confirmed", 1, 1, seriesNow().Add(-time.Hour))
	noRatio := func(_ context.Context, keys []string, _ time.Time) (map[string]app.ListingQuote, error) {
		out := map[string]app.ListingQuote{}
		for _, key := range keys {
			out[key] = app.ListingQuote{
				Symbol: "AAPLon", Network: chain.NetworkBase, Decimals: 6,
				Asset: &app.ListedAsset{Mint: perfMint, Symbol: "AAPLx", DisplayName: "Apple"},
			}
		}
		return out, nil
	}
	reads := app.NewHoldingPerformanceReads(pool, member(true), perfNames{}, noRatio, down.read, fixedNow{})
	if _, err := reads.Handle(t.Context(), query); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("missing shares ratio = %v, want internal", err)
	}
}
