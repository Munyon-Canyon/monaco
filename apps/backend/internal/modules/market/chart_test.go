package market_test

import (
	"net/http"
	"testing"
	"time"

	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/marketapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func chartHourNow() time.Time { return time.Date(2026, 3, 4, 15, 0, 30, 0, time.UTC) }

func seedHourSamples(t *testing.T, s marketAPI, mint string) {
	t.Helper()
	s.price(t, mint, -time.Hour-20*time.Second, 90_000_000)
	for k := range 30 {
		s.price(t, mint, -time.Hour+10*time.Second+time.Duration(k)*120*time.Second, int64(100_000_000+k*1_000_000))
	}
}

func TestChart_oneHourKeepsOnlyBarsStartingInsideTheWindow(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, chartHourNow())
	s.seedFixtures(t)
	seedHourSamples(t, s, marketfake.AAPLx().Mint.String())
	got := chartOf(t, s.get(t, "/v1/assets/AAPLx/chart?range=1H"))
	if got.Range != api.AssetChartRangeN1H || got.Interval != api.AssetChartIntervalN1m || got.BucketSeconds != 60 {
		t.Fatalf("1H = %+v", got)
	}
	if got.Empty || len(got.Points) != 29 || got.PreviousCloseMicros != nil {
		t.Fatalf("1H bars = %d empty %v base %v", len(got.Points), got.Empty, got.PreviousCloseMicros)
	}
	windowStart := chartHourNow().Add(-time.Hour)
	for i, p := range got.Points {
		want := time.Date(2026, 3, 4, 14, 2+2*i, 0, 0, time.UTC)
		if p.T.Before(windowStart) || !p.T.Equal(want) {
			t.Fatalf("bar %d starts %s, want %s and not before %s", i, p.T, want, windowStart)
		}
		price := int64(100_000_000 + (i+1)*1_000_000)
		assertOHLC(t, p, price, price, price, price)
	}
}

func TestChart_oneHourAtFiveMinutesStartsOnTheFirstWholeBarInsideTheWindow(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, chartHourNow())
	s.seedFixtures(t)
	seedHourSamples(t, s, marketfake.AAPLx().Mint.String())
	got := chartOf(t, s.get(t, "/v1/assets/AAPLx/chart?range=1H&interval=5m"))
	if got.BucketSeconds != 300 || len(got.Points) != 11 {
		t.Fatalf("1H 5m = %d bars, bucket %d", len(got.Points), got.BucketSeconds)
	}
	if first := got.Points[0]; !first.T.Equal(time.Date(2026, 3, 4, 14, 5, 0, 0, time.UTC)) ||
		first.OpenMicros != 103_000_000 {
		t.Fatalf("first = %+v", first)
	}
}

func TestChart_aMinuteBarIsRefusedOutsideOneHour(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, chartHourNow())
	s.seedFixtures(t)
	for _, path := range []string{
		"/v1/assets/AAPLx/chart?range=1D&interval=1m",
		"/v1/assets/AAPLx/chart?range=1W&interval=1m",
		"/v1/assets/AAPLx/chart?range=1H&interval=15m",
	} {
		rec := s.get(t, path)
		if rec.Code != http.StatusBadRequest || problemCode(t, rec) != apibase.InvalidInput {
			t.Fatalf("%s = %d %s", path, rec.Code, rec.Body)
		}
	}
}
