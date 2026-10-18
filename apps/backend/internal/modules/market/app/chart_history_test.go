package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type fakeHistory struct {
	mu      sync.Mutex
	samples []Sample
	err     error
	off     bool
	days    []int
}

func (f *fakeHistory) Configured() bool { return !f.off }

func (f *fakeHistory) MarketChart(_ context.Context, _ domain.Mint, days int) ([]Sample, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.days = append(f.days, days)
	return f.samples, f.err
}

func (f *fakeHistory) calls() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.days
}

func usd(v uint64) money.Micros { return money.MicrosFromUint64(v * 1_000_000) }

func historyWhen() time.Time { return time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC) }

func historySamples() []Sample {
	at := historyWhen()
	return []Sample{
		{At: at.Add(-50 * time.Minute), Price: usd(10)},
		{At: at.Add(-20 * time.Minute), Price: usd(12)},
		{At: at.Add(-10 * time.Minute), Price: usd(8)},
		{At: at.Add(-5 * time.Minute), Price: usd(9)},
		{At: at.Add(-3 * 24 * time.Hour), Price: usd(7)},
	}
}

func historyChart(t *testing.T, hist PriceHistory, read chartRead) *AssetChart {
	t.Helper()
	return (&AssetChart{read: read, base: read, clock: testkit.NewClock(historyWhen())}).WithHistory(hist)
}

func onlyAsset(t *testing.T) chartRead {
	t.Helper()
	apple := appleRow(t, "equity")
	return chartRead{asset: func() (sqlc.Asset, error) { return apple, nil }}
}

func wantPoints(t *testing.T, got, want []Point) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("points = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("point %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestChart_aWeekChartIsBucketedFromTheHistorySamples(t *testing.T) {
	t.Parallel()
	hist := &fakeHistory{samples: historySamples()}
	chart := historyChart(t, hist, onlyAsset(t))
	got, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: "1W"})
	if err != nil || got.Empty || got.Bucket != time.Hour || got.NextBefore != nil || got.PreviousClose != nil {
		t.Fatalf("chart = %+v err %v", got, err)
	}
	at := historyWhen()
	wantPoints(t, got.Points, []Point{
		{At: at.Add(-3 * 24 * time.Hour), Open: usd(7), High: usd(7), Low: usd(7), Close: usd(7)},
		{At: at.Add(-time.Hour), Open: usd(10), High: usd(12), Low: usd(8), Close: usd(9)},
	})
	if calls := hist.calls(); len(calls) != 1 || calls[0] != 7 {
		t.Fatalf("history calls = %v", calls)
	}
}

func TestChart_eachRangeAsksForItsSpanInDays(t *testing.T) {
	t.Parallel()
	want := map[string]int{
		"1D": 1, "1W": 7, "1M": 30, "3M": 90, "6M": 180, "1Y": 365, "ALL": HistoryMaxDays,
	}
	for raw, days := range want {
		hist := &fakeHistory{samples: historySamples()}
		read := onlyAsset(t)
		read.newestAt = func(time.Time) []sqlc.NewestSamplesRow { return nil }
		chart := historyChart(t, hist, read)
		if _, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: raw}); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if calls := hist.calls(); len(calls) != 1 || calls[0] != days {
			t.Fatalf("%s asked for %v, want %d", raw, calls, days)
		}
	}
}

func TestChart_theHistoryWindowDropsSamplesOlderThanTheRange(t *testing.T) {
	t.Parallel()
	samples := append(historySamples(), Sample{At: historyWhen().Add(-8 * 24 * time.Hour), Price: usd(5)})
	hist := &fakeHistory{samples: samples}
	chart := historyChart(t, hist, onlyAsset(t))
	got, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: "1W"})
	if err != nil || len(got.Points) != 2 || got.Points[0].Close != usd(7) {
		t.Fatalf("chart = %+v err %v", got, err)
	}
	if got.NextBefore == nil || !got.NextBefore.Equal(got.Points[0].At) {
		t.Fatalf("next before = %v", got.NextBefore)
	}
}

func TestChart_unsortedHistoryIsBucketedInTimeOrder(t *testing.T) {
	t.Parallel()
	samples := historySamples()
	samples[0], samples[4] = samples[4], samples[0]
	hist := &fakeHistory{samples: samples}
	chart := historyChart(t, hist, onlyAsset(t))
	got, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: "1W"})
	if err != nil || len(got.Points) != 2 || !got.Points[0].At.Before(got.Points[1].At) ||
		got.Points[1].Open != usd(10) {
		t.Fatalf("chart = %+v err %v", got, err)
	}
}

func TestChart_aHistoryPageStopsBeforeTheCursorAndLinksTheOlderSamples(t *testing.T) {
	t.Parallel()
	hist := &fakeHistory{samples: historySamples()}
	chart := historyChart(t, hist, onlyAsset(t))
	before := historyWhen()
	got, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: "1W", Before: &before, Limit: 1})
	if err != nil || len(got.Points) != 1 || got.Points[0].Close != usd(9) {
		t.Fatalf("chart = %+v err %v", got, err)
	}
	if got.NextBefore == nil || !got.NextBefore.Equal(got.Points[0].At) {
		t.Fatalf("next before = %v", got.NextBefore)
	}
	older := got.Points[0].At
	last, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: "1W", Before: &older, Limit: 5})
	if err != nil || len(last.Points) != 1 || last.Points[0].Close != usd(7) || last.NextBefore != nil {
		t.Fatalf("last page = %+v err %v", last, err)
	}
}

func TestChart_aDayChartFromHistoryCarriesThePreviousClose(t *testing.T) {
	t.Parallel()
	apple := appleRow(t, "equity")
	when := historyWhen()
	info, err := domain.Session(domain.KindEquity, when)
	if err != nil {
		t.Fatal(err)
	}
	read := chartRead{
		asset: func() (sqlc.Asset, error) { return apple, nil },
		newestAt: func(at time.Time) []sqlc.NewestSamplesRow {
			if at.Equal(info.LastClose) {
				return []sqlc.NewestSamplesRow{
					{Mint: apple.Mint, Ts: info.LastClose.Add(-time.Minute), PriceMicros: 98_000_000},
				}
			}
			return nil
		},
	}
	hist := &fakeHistory{samples: historySamples()}
	got, err := historyChart(t, hist, read).Handle(t.Context(), "AAPLx", ChartRequest{Range: "1D"})
	if err != nil || got.PreviousClose == nil || *got.PreviousClose != money.MicrosFromUint64(98_000_000) {
		t.Fatalf("previous close = %v err %v", got.PreviousClose, err)
	}
	if len(got.Points) != 4 || got.Points[0].Open != usd(10) {
		t.Fatalf("points = %+v", got.Points)
	}
}

func TestChart_aDayChartFromHistoryReportsAPreviousCloseFailure(t *testing.T) {
	t.Parallel()
	apple := appleRow(t, "equity")
	boom := errs.New(errs.CodeInternal, "market.test")
	read := chartRead{asset: func() (sqlc.Asset, error) { return apple, nil }}
	chart := (&AssetChart{read: read, base: failingBase{boom}, clock: testkit.NewClock(historyWhen())}).
		WithHistory(&fakeHistory{samples: historySamples()})
	if _, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: "1D"}); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("err = %v", err)
	}
}

func TestChart_aMissingUnconfiguredOrFailingHistoryFallsBackToPricePoints(t *testing.T) {
	t.Parallel()
	apple := appleRow(t, "equity")
	when := historyWhen()
	read := chartRead{
		asset: func() (sqlc.Asset, error) { return apple, nil },
		newest: func() ([]sqlc.NewestSamplesRow, error) {
			return []sqlc.NewestSamplesRow{{Mint: apple.Mint, Ts: when.Add(-time.Minute), PriceMicros: 100}}, nil
		},
		buckets: func() ([]sqlc.ChartBucketsPageRow, error) {
			return []sqlc.ChartBucketsPageRow{{
				Bucket: when.Add(-time.Hour), OpenMicros: 1, HighMicros: 4, LowMicros: 1, CloseMicros: 3,
			}}, nil
		},
	}
	cases := map[string]*fakeHistory{
		"not found":    {err: errs.New(errs.CodeNotFound, "coingecko.test")},
		"upstream":     {err: errs.New(errs.CodeUpstreamUnavailable, "coingecko.test")},
		"empty":        {},
		"unconfigured": {off: true, samples: historySamples()},
	}
	for name, hist := range cases {
		got, err := historyChart(t, hist, read).Handle(t.Context(), "AAPLx", ChartRequest{Range: "1W"})
		if err != nil || len(got.Points) != 1 || got.Points[0].High.Uint64() != 4 {
			t.Fatalf("%s: chart = %+v err %v", name, got, err)
		}
	}
	if len(cases["unconfigured"].calls()) != 0 {
		t.Fatal("an unconfigured history was called")
	}
}

func TestChart_anHourRangeIsRefusedBeforeAnyHistoryCall(t *testing.T) {
	t.Parallel()
	hist := &fakeHistory{samples: historySamples()}
	_, err := historyChart(t, hist, onlyAsset(t)).Handle(t.Context(), "AAPLx", ChartRequest{Range: "1H"})
	if errs.CodeOf(err) != errs.CodeInvalidInput || len(hist.calls()) != 0 {
		t.Fatalf("err = %v calls %v", err, hist.calls())
	}
}

type failingBase struct{ err error }

func (f failingBase) FirstSamplesSince(
	context.Context,
	sqlc.FirstSamplesSinceParams,
) ([]sqlc.FirstSamplesSinceRow, error) {
	return nil, f.err
}

func (f failingBase) NewestSamples(context.Context, sqlc.NewestSamplesParams) ([]sqlc.NewestSamplesRow, error) {
	return nil, f.err
}

func TestChart_aClosedMarketDayChartFromHistoryStartsAtTheLastSessionOpen(t *testing.T) {
	t.Parallel()
	saturday := time.Date(2026, 3, 7, 15, 0, 0, 0, time.UTC)
	fridayOpen := time.Date(2026, 3, 6, 14, 30, 0, 0, time.UTC)
	read := onlyAsset(t)
	read.newestAt = func(time.Time) []sqlc.NewestSamplesRow { return nil }
	hist := &fakeHistory{samples: []Sample{
		{At: fridayOpen.Add(-time.Hour), Price: usd(3)},
		{At: fridayOpen.Add(time.Hour), Price: usd(6)},
		{At: fridayOpen.Add(70 * time.Minute), Price: usd(4)},
	}}
	chart := (&AssetChart{read: read, base: read, clock: testkit.NewClock(saturday)}).WithHistory(hist)
	got, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: "1D"})
	if err != nil || len(got.Points) != 2 || got.Points[0].Open != usd(6) || got.Points[1].Low != usd(4) {
		t.Fatalf("chart = %+v err %v", got, err)
	}
}

func TestChart_historyEntirelyOutsideTheWindowIsAnEmptyChart(t *testing.T) {
	t.Parallel()
	hist := &fakeHistory{samples: []Sample{{At: historyWhen().Add(-8 * 24 * time.Hour), Price: usd(5)}}}
	got, err := historyChart(t, hist, onlyAsset(t)).Handle(t.Context(), "AAPLx", ChartRequest{Range: "1W"})
	if err != nil || !got.Empty || got.NextBefore != nil || len(got.Points) != 0 {
		t.Fatalf("chart = %+v err %v", got, err)
	}
}
