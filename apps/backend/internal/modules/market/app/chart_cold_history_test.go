package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
)

type peekHistory struct {
	fakeHistory
	warm  bool
	peeks atomic.Int64
}

func (p *peekHistory) Peek(context.Context, domain.Mint, int) (History, bool, error) {
	p.peeks.Add(1)
	if p.warm {
		return History{Prices: p.samples}, true, nil
	}
	return History{}, false, nil
}

func coldRead(t *testing.T, firstBar time.Duration) (chartRead, *atomic.Int64) {
	t.Helper()
	apple := appleRow(t, "equity")
	when := historyWhen()
	var reads atomic.Int64
	return chartRead{
		asset: func() (sqlc.Asset, error) { return apple, nil },
		newestAt: func(time.Time) []sqlc.NewestSamplesRow {
			return []sqlc.NewestSamplesRow{{Mint: apple.Mint, Ts: when.Add(-time.Minute), PriceMicros: 9_000_000}}
		},
		buckets: func() ([]sqlc.ChartBucketsPageRow, error) {
			reads.Add(1)
			return []sqlc.ChartBucketsPageRow{
				{
					Bucket: when.Add(-firstBar), OpenMicros: 7_000_000, HighMicros: 7_000_000,
					LowMicros: 7_000_000, CloseMicros: 7_000_000,
				},
				{
					Bucket: when.Add(-time.Hour), OpenMicros: 8_000_000, HighMicros: 8_000_000,
					LowMicros: 8_000_000, CloseMicros: 8_000_000,
				},
			}, nil
		},
	}, &reads
}

func TestChart_aColdHistoryAnswersFromThePricePointsWithoutWaiting(t *testing.T) {
	t.Parallel()
	hist := &peekHistory{fakeHistory: fakeHistory{samples: historySamples()}}
	read, reads := coldRead(t, 23*time.Hour+30*time.Minute)
	got, err := historyChart(t, hist, read).Handle(t.Context(), "AAPLx", ChartRequest{Range: "1D"})
	if err != nil || got.Empty || len(got.Points) != 2 || got.Points[0].Close != usd(7) {
		t.Fatalf("chart = %+v err %v", got, err)
	}
	if calls := hist.calls(); len(calls) != 0 {
		t.Fatalf("a cold chart waited on the history: %v", calls)
	}
	if hist.peeks.Load() != 1 || reads.Load() != 1 {
		t.Fatalf("peeks %d, price point reads %d", hist.peeks.Load(), reads.Load())
	}
}

func TestChart_aColdHistoryThePricePointsDoNotCoverStillWaitsForIt(t *testing.T) {
	t.Parallel()
	hist := &peekHistory{fakeHistory: fakeHistory{samples: historySamples()}}
	read, _ := coldRead(t, 3*time.Hour)
	got, err := historyChart(t, hist, read).Handle(t.Context(), "AAPLx", ChartRequest{Range: "1D"})
	if err != nil || len(got.Points) != 4 {
		t.Fatalf("chart = %+v err %v", got, err)
	}
	if calls := hist.calls(); len(calls) != 1 || calls[0] != 1 {
		t.Fatalf("history calls = %v, want one for the day", calls)
	}
}

func TestChart_aWarmHistoryIsServedWithoutReadingThePricePoints(t *testing.T) {
	t.Parallel()
	hist := &peekHistory{fakeHistory: fakeHistory{samples: historySamples()}, warm: true}
	read, reads := coldRead(t, 23*time.Hour+30*time.Minute)
	got, err := historyChart(t, hist, read).Handle(t.Context(), "AAPLx", ChartRequest{Range: "1D"})
	if err != nil || len(got.Points) != 4 || reads.Load() != 0 {
		t.Fatalf("chart = %+v reads %d err %v", got, reads.Load(), err)
	}
}

func TestChart_aPagedColdHistoryWaitsForIt(t *testing.T) {
	t.Parallel()
	hist := &peekHistory{fakeHistory: fakeHistory{samples: historySamples()}}
	read, _ := coldRead(t, 23*time.Hour+30*time.Minute)
	before := historyWhen()
	got, err := historyChart(t, hist, read).Handle(t.Context(), "AAPLx", ChartRequest{Range: "1W", Before: &before})
	if err != nil || len(got.Points) == 0 {
		t.Fatalf("chart = %+v err %v", got, err)
	}
	if hist.peeks.Load() != 0 || len(hist.calls()) != 1 {
		t.Fatalf("peeks %d, history calls %v", hist.peeks.Load(), hist.calls())
	}
}

func TestChart_aColdAllRangeWaitsForTheHistory(t *testing.T) {
	t.Parallel()
	hist := &peekHistory{fakeHistory: fakeHistory{samples: historySamples()}}
	read, _ := coldRead(t, 23*time.Hour+30*time.Minute)
	read.earliest = func() (time.Time, error) { return historyWhen().Add(-24 * time.Hour), nil }
	if _, err := historyChart(t, hist, read).Handle(t.Context(), "AAPLx", ChartRequest{Range: "ALL"}); err != nil {
		t.Fatal(err)
	}
	if calls := hist.calls(); len(calls) != 1 || calls[0] != HistoryMaxDays {
		t.Fatalf("history calls = %v", calls)
	}
}

func TestChart_aColdHistoryReportsAPricePointReadFailure(t *testing.T) {
	t.Parallel()
	hist := &peekHistory{fakeHistory: fakeHistory{samples: historySamples()}}
	read, _ := coldRead(t, time.Hour)
	boom := errs.New(errs.CodeInternal, "market.test")
	read.buckets = func() ([]sqlc.ChartBucketsPageRow, error) { return nil, boom }
	_, err := historyChart(t, hist, read).Handle(t.Context(), "AAPLx", ChartRequest{Range: "1D"})
	if errs.CodeOf(err) != errs.CodeInternal || len(hist.calls()) != 0 {
		t.Fatalf("err = %v, history calls %v", err, hist.calls())
	}
}

func TestChart_aColdHistoryWithNoPricePointsWaitsForIt(t *testing.T) {
	t.Parallel()
	hist := &peekHistory{fakeHistory: fakeHistory{samples: historySamples()}}
	read, _ := coldRead(t, time.Hour)
	read.buckets = func() ([]sqlc.ChartBucketsPageRow, error) { return nil, nil }
	got, err := historyChart(t, hist, read).Handle(t.Context(), "AAPLx", ChartRequest{Range: "1D"})
	if err != nil || len(got.Points) != 4 || len(hist.calls()) != 1 {
		t.Fatalf("chart = %+v err %v calls %v", got, err, hist.calls())
	}
}
