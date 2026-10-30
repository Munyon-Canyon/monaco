package market_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestPositionPrices_readsTheChartClosesOfTheSharedChart(t *testing.T) {
	t.Parallel()
	var mod *market.Module
	s := newMarketAPIMounting(t, chartHourNow(), func(d module.Deps) func(apibase.Mount) {
		mod = market.New(d)
		return mod.Mount
	})
	s.seedFixtures(t)
	seedHourSamples(t, s, marketfake.AAPLx().Mint.String())
	got, err := mod.PositionPrices().PositionPrices(t.Context(), "AAPLx", "1W")
	if err != nil || len(got) == 0 || got[len(got)-1].At.After(chartHourNow()) || got[0].PriceMicros.Uint64() == 0 {
		t.Fatalf("1W closes = %+v, %v, want priced buckets up to now", got, err)
	}
	if _, err := mod.PositionPrices().PositionPrices(t.Context(), "NOPE", "1W"); err == nil {
		t.Fatal("unknown symbol read nothing wrong, want an error")
	}
}
