package marketfake_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestPriceHistoryFake_answersWhatWasPutPerMintAndDaysAndRecordsCalls(t *testing.T) {
	t.Parallel()
	aapl, tsla := marketfake.AAPLx().Mint, marketfake.TSLAx().Mint
	sample := app.Sample{At: clock.Real{}.Now().Truncate(time.Hour), Price: money.MicrosFromUint64(210_000_000)}
	var f marketfake.PriceHistoryFake
	f.Put(aapl, 90, sample)
	got, err := f.MarketChart(t.Context(), aapl, 90)
	if err != nil || len(got) != 1 || got[0] != sample {
		t.Fatalf("MarketChart(AAPLx, 90) = %v, %v, want the put sample", got, err)
	}
	for _, other := range []struct {
		mint market.Mint
		days int
	}{{aapl, 1}, {tsla, 90}} {
		if got, err := f.MarketChart(t.Context(), other.mint, other.days); err != nil || len(got) != 0 {
			t.Fatalf("MarketChart(%v, %d) = %v, %v, want an empty answer", other.mint, other.days, got, err)
		}
	}
	if calls := f.Calls(); len(calls) != 3 || calls[0].Mint != aapl || calls[0].Days != 90 || calls[2].Mint != tsla {
		t.Fatalf("Calls = %v, want the three calls in order", calls)
	}
}

func TestPriceHistoryFake_isConfiguredUntilItLosesItsKey(t *testing.T) {
	t.Parallel()
	var f marketfake.PriceHistoryFake
	if !f.Configured() {
		t.Fatal("a new fake is not configured")
	}
	f.WithoutKey()
	if f.Configured() {
		t.Fatal("a fake without a key is configured")
	}
}

func TestPriceHistoryFake_runsTheHookAndFailsWhereFaultsSay(t *testing.T) {
	t.Parallel()
	var f marketfake.PriceHistoryFake
	hooked := 0
	f.During(func() { hooked++ })
	f.FailOnce("MarketChart", errs.New(errs.CodeCoinGeckoRateLimited, "test"))
	aapl := marketfake.AAPLx().Mint
	if _, err := f.MarketChart(t.Context(), aapl, 1); errs.CodeOf(err) != errs.CodeCoinGeckoRateLimited {
		t.Fatalf("MarketChart err = %v, want the scripted fault", err)
	}
	if _, err := f.MarketChart(t.Context(), aapl, 1); err != nil || hooked != 2 {
		t.Fatalf("after the fault err = %v with %d hook runs, want nil and 2", err, hooked)
	}
}
