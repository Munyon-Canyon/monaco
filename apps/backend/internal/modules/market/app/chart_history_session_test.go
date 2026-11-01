package app

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func micros(v uint64) money.Micros { return money.MicrosFromUint64(v) }

func sessionChart(t *testing.T, now time.Time, headlineAt time.Time, headline int64, hist PriceHistory) *AssetChart {
	t.Helper()
	apple := appleRow(t, "equity")
	read := chartRead{
		asset: func() (sqlc.Asset, error) { return apple, nil },
		newestAt: func(time.Time) []sqlc.NewestSamplesRow {
			return []sqlc.NewestSamplesRow{{Mint: apple.Mint, Ts: headlineAt, PriceMicros: headline}}
		},
	}
	return (&AssetChart{read: read, base: read, clock: testkit.NewClock(now)}).WithHistory(hist)
}

func TestChart_aSaturdayHistoryChartCoversFridaysSessionAndEndsAtTheHeadline(t *testing.T) {
	t.Parallel()
	saturday := time.Date(2026, 3, 7, 15, 0, 0, 0, time.UTC)
	headlineAt := saturday.Add(-time.Minute)
	hist := &fakeHistory{samples: []Sample{
		{At: time.Date(2026, 3, 6, 14, 0, 0, 0, time.UTC), Price: micros(150_000_000)},
		{At: time.Date(2026, 3, 6, 15, 0, 0, 0, time.UTC), Price: micros(190_000_000)},
		{At: time.Date(2026, 3, 6, 20, 55, 0, 0, time.UTC), Price: micros(200_000_000)},
		{At: saturday.Add(-time.Hour), Price: micros(201_000_000)},
		{At: saturday.Add(-30 * time.Second), Price: micros(210_000_000)},
	}}
	got, err := sessionChart(t, saturday, headlineAt, 200_400_000, hist).Handle(
		t.Context(), "AAPLx", ChartRequest{Range: "1D"})
	if err != nil || got.Empty || len(got.Points) == 0 {
		t.Fatalf("chart = %+v err %v", got, err)
	}
	if calls := hist.calls(); len(calls) != 1 || calls[0] != 2 {
		t.Fatalf("history days = %v, want [2] to reach Friday's open", calls)
	}
	first, last := got.Points[0], got.Points[len(got.Points)-1]
	if first.At.Before(time.Date(2026, 3, 6, 14, 30, 0, 0, time.UTC)) || first.Open != micros(190_000_000) {
		t.Fatalf("first point = %+v, want the first sample of Friday's session", first)
	}
	if last.Close != micros(200_400_000) || last.At.After(headlineAt) {
		t.Fatalf("last point = %+v, want the headline price at or before %s", last, headlineAt)
	}
}

func TestChart_aHolidayHistoryChartAsksForEnoughDaysToReachTheLastSessionOpen(t *testing.T) {
	t.Parallel()
	holiday := time.Date(2026, 1, 19, 17, 0, 0, 0, time.UTC)
	hist := &fakeHistory{
		samples: []Sample{{At: time.Date(2026, 1, 16, 15, 0, 0, 0, time.UTC), Price: micros(190_000_000)}},
	}
	got, err := sessionChart(t, holiday, holiday.Add(-time.Minute), 200_000_000, hist).Handle(
		t.Context(), "AAPLx", ChartRequest{Range: "1D"})
	if err != nil || len(got.Points) != 2 || got.Points[0].Open != micros(190_000_000) {
		t.Fatalf("chart = %+v err %v", got, err)
	}
	if calls := hist.calls(); len(calls) != 1 || calls[0] != 4 {
		t.Fatalf("history days = %v, want [4]", calls)
	}
}

func TestChart_anOpenDayHistoryChartKeepsAskingForOneDay(t *testing.T) {
	t.Parallel()
	monday := time.Date(2026, 3, 9, 15, 0, 0, 0, time.UTC)
	hist := &fakeHistory{samples: []Sample{{At: monday.Add(-time.Hour), Price: micros(190_000_000)}}}
	if _, err := sessionChart(t, monday, monday.Add(-time.Minute), 191_000_000, hist).Handle(
		t.Context(), "AAPLx", ChartRequest{Range: "1D"}); err != nil {
		t.Fatal(err)
	}
	if calls := hist.calls(); len(calls) != 1 || calls[0] != 1 {
		t.Fatalf("history days = %v, want [1]", calls)
	}
}

func TestChart_aHistorySpikeIsDroppedAndASampleAfterTheHeadlineNeverShows(t *testing.T) {
	t.Parallel()
	monday := time.Date(2026, 3, 9, 15, 0, 0, 0, time.UTC)
	hist := &fakeHistory{samples: []Sample{
		{At: monday.Add(-50 * time.Minute), Price: micros(190_000_000)},
		{At: monday.Add(-40 * time.Minute), Price: micros(900_000_000)},
		{At: monday.Add(-30 * time.Minute), Price: micros(191_000_000)},
		{At: monday.Add(-3 * time.Minute), Price: micros(192_000_000)},
		{At: monday.Add(-time.Minute), Price: micros(260_000_000)},
	}}
	got, err := sessionChart(t, monday, monday.Add(-2*time.Minute), 192_500_000, hist).Handle(
		t.Context(), "AAPLx", ChartRequest{Range: "1D"})
	if err != nil {
		t.Fatal(err)
	}
	var high money.Micros
	for _, p := range got.Points {
		if p.High.Cmp(high) > 0 {
			high = p.High
		}
	}
	last := got.Points[len(got.Points)-1]
	if high != micros(192_500_000) || last.Close != micros(192_500_000) {
		t.Fatalf("points = %+v, want no spike and the headline last", got.Points)
	}
}

func TestChart_aBeforePageOfHistoryIsNotTiedToTheHeadline(t *testing.T) {
	t.Parallel()
	monday := time.Date(2026, 3, 9, 15, 0, 0, 0, time.UTC)
	before := monday.Add(-time.Hour)
	hist := &fakeHistory{samples: []Sample{{At: monday.Add(-2 * time.Hour), Price: micros(190_000_000)}}}
	got, err := sessionChart(t, monday, monday.Add(-time.Minute), 250_000_000, hist).Handle(
		t.Context(), "AAPLx", ChartRequest{Range: "1W", Before: &before})
	if err != nil || len(got.Points) != 1 || got.Points[0].Close != micros(190_000_000) {
		t.Fatalf("chart = %+v err %v", got, err)
	}
}
