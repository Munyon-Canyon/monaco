package marketfake_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestPricesFake_answersTheNewestPriceOverallOrAtATime(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now()
	aapl, tsla, jpst := marketfake.AAPLx().ID, marketfake.TSLAx().ID, marketfake.JPSTx().ID
	var f marketfake.PricesFake
	f.Set(aapl, money.MicrosFromUint64(210_000_000), now)
	f.Set(aapl, money.MicrosFromUint64(200_000_000), now.Add(-time.Hour))
	f.Set(tsla, money.MicrosFromUint64(300_000_000), now.Add(-time.Minute))
	latest, err := f.LatestPrices(t.Context())
	if err != nil || len(latest) != 2 || latest[aapl].Micros.Uint64() != 210_000_000 ||
		!latest[tsla].ObservedAt.Equal(now.Add(-time.Minute)) {
		t.Fatalf("LatestPrices = %v, %v, want AAPLx at 210 and TSLAx a minute ago", latest, err)
	}
	asOf, err := f.PricesAsOf(t.Context(), []market.AssetID{aapl, jpst}, now.Add(-30*time.Minute))
	if err != nil || len(asOf) != 1 || asOf[aapl].Micros.Uint64() != 200_000_000 {
		t.Fatalf("PricesAsOf half an hour ago = %v, %v, want only AAPLx at 200", asOf, err)
	}
}

func TestPricesFake_failsWhereFaultsSay(t *testing.T) {
	t.Parallel()
	var f marketfake.PricesFake
	f.FailOnce("LatestPrices", errs.New(errs.CodeDBUnavailable, "test"))
	if _, err := f.LatestPrices(t.Context()); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("LatestPrices err = %v, want the scripted fault", err)
	}
	f.FailOnce("PricesAsOf", errs.New(errs.CodeDBUnavailable, "test"))
	if _, err := f.PricesAsOf(t.Context(), nil, clock.Real{}.Now()); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("PricesAsOf err = %v, want the scripted fault", err)
	}
	f.FailOnce("CurrentPricesAsOf", errs.New(errs.CodeDBUnavailable, "test"))
	if _, err := f.CurrentPricesAsOf(t.Context(), nil, clock.Real{}.Now()); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("CurrentPricesAsOf err = %v, want the scripted fault", err)
	}
}

func TestPricesFake_currentPricesSkipANewestSampleTheLeaderboardsWouldHold(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now()
	aapl := marketfake.AAPLx().ID
	var f marketfake.PricesFake
	f.Set(aapl, money.MicrosFromUint64(100_000_000), now.Add(-4*time.Minute))
	f.Set(aapl, money.MicrosFromUint64(100_000_000), now.Add(-2*time.Minute))
	f.Set(aapl, money.MicrosFromUint64(60_000_000), now)
	got, err := f.CurrentPricesAsOf(t.Context(), []market.AssetID{aapl}, now)
	if err != nil || len(got) != 0 {
		t.Fatalf("CurrentPricesAsOf with a held 40%% drop = %v, %v, want no price", got, err)
	}
	f.Set(aapl, money.MicrosFromUint64(60_000_000), now.Add(2*time.Minute))
	got, err = f.CurrentPricesAsOf(t.Context(), []market.AssetID{aapl}, now.Add(2*time.Minute))
	if err != nil || got[aapl].Micros.Uint64() != 60_000_000 {
		t.Fatalf("CurrentPricesAsOf once confirmed = %v, %v, want 60", got, err)
	}
}
