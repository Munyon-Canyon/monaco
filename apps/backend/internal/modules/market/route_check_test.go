package market_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/jupiterquote"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const (
	orderRoute = "/jupiter/swap/v2/order"
	usdcMint   = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
)

func quotedAt() time.Time { return time.Date(2026, 10, 2, 15, 0, 0, 0, time.UTC) }

type quoteCalls struct {
	srv   *fakes.Server
	calls atomic.Int32

	mu    sync.Mutex
	query url.Values
}

func (c *quoteCalls) RoundTrip(r *http.Request) (*http.Response, error) {
	c.calls.Add(1)
	c.mu.Lock()
	c.query = r.URL.Query()
	c.mu.Unlock()
	rec := httptest.NewRecorder()
	c.srv.ServeHTTP(rec, r)
	return rec.Result(), nil
}

func (c *quoteCalls) asked() url.Values {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.query
}

type sampledPrice struct {
	micros uint64
	err    error
}

func (p sampledPrice) PricesAsOf(
	_ context.Context, ids []domain.AssetID, at time.Time,
) (map[domain.AssetID]domain.Sample, error) {
	out := map[domain.AssetID]domain.Sample{}
	if p.err != nil || p.micros == 0 {
		return out, p.err
	}
	for _, id := range ids {
		out[id] = domain.Sample{Micros: money.MicrosFromUint64(p.micros), ObservedAt: at}
	}
	return out, nil
}

func routeChecker(t *testing.T, assets ...market.Asset) (*app.RouteChecker, *quoteCalls) {
	t.Helper()
	return pricedRouteChecker(t, sampledPrice{}, assets...)
}

func pricedRouteChecker(
	t *testing.T, prices app.SampledPrices, assets ...market.Asset,
) (*app.RouteChecker, *quoteCalls) {
	t.Helper()
	calls := &quoteCalls{srv: fakes.New()}
	cfg := config.Config{
		Jupiter: config.Jupiter{
			SwapBaseURL:  "http://jupiter.test/jupiter/swap/v2",
			PriceBaseURL: "http://jupiter.test/jupiter/price/v3",
			APIKey:       "test-key",
		},
		Timeouts: config.Timeouts{JupiterQuote: 5 * time.Second, JupiterExecute: time.Minute},
	}
	client := jupiter.New(cfg, clock.Real{}, httpclient.WithTransport(calls))
	checker := app.NewRouteChecker(
		marketfake.NewCatalog(assets...), prices, jupiterquote.New(client), testkit.NewClock(quotedAt()),
	)
	return checker, calls
}

func scriptQuote(t *testing.T, srv *fakes.Server, step fakes.Step) {
	t.Helper()
	raw, err := json.Marshal(step)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/_script", bytes.NewReader(raw))
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("script %s = %d %q", raw, rec.Code, rec.Body.String())
	}
}

func wantCheck(t *testing.T, got app.RouteCheck, err error, want app.RouteCheck) {
	t.Helper()
	if err != nil || got != want {
		t.Fatalf("CheckRoute = %+v, %v, want %+v", got, err, want)
	}
}

func wantCode(t *testing.T, err error, code errs.Code, calls *quoteCalls, n int32) {
	t.Helper()
	if errs.CodeOf(err) != code || calls.calls.Load() != n {
		t.Fatalf("err = %v after %d quotes, want %s after %d", err, calls.calls.Load(), code, n)
	}
}

func TestCheckRoute_Ok_Buy(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	checker, calls := routeChecker(t, aapl)
	got, err := checker.CheckRoute(t.Context(), aapl.ID, app.SideBuy, money.NewBaseUnits(25_000_000, 6))
	wantCheck(t, got, err, app.RouteCheck{
		InAmount:       money.NewBaseUnits(25_000_000, 6),
		OutAmount:      money.NewBaseUnits(11_000_000, 8),
		PriceImpactBps: 12,
		QuotedAt:       quotedAt(),
	})
	q := calls.asked()
	if calls.calls.Load() != 1 || q.Has("taker") || q.Get("inputMint") != usdcMint ||
		q.Get("outputMint") != aapl.Mint.String() || q.Get("amount") != "25000000" {
		t.Fatalf("calls = %d query = %v, want one taker-less USDC to AAPLx quote", calls.calls.Load(), q)
	}
}

func TestCheckRoute_Ok_Sell(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	checker, calls := routeChecker(t, aapl)
	scriptQuote(t, calls.srv, fakes.Step{Route: orderRoute, Action: fakes.ActionSucceed, Fixture: orderRoute + "/sell"})
	got, err := checker.CheckRoute(t.Context(), aapl.ID, app.SideSell, money.NewBaseUnits(11_000_000, 8))
	wantCheck(t, got, err, app.RouteCheck{
		InAmount:       money.NewBaseUnits(11_000_000, 8),
		OutAmount:      money.NewBaseUnits(24_870_000, 6),
		PriceImpactBps: 9,
		QuotedAt:       quotedAt(),
	})
	q := calls.asked()
	if calls.calls.Load() != 1 || q.Has("taker") || q.Get("inputMint") != aapl.Mint.String() ||
		q.Get("outputMint") != usdcMint {
		t.Fatalf("calls = %d query = %v, want one taker-less AAPLx to USDC quote", calls.calls.Load(), q)
	}
}

func noRouteFor(t *testing.T, calls *quoteCalls, amount string) {
	t.Helper()
	scriptQuote(t, calls.srv, fakes.Step{
		Route: orderRoute, Action: fakes.ActionSucceed, Fixture: orderRoute + "/no-route",
		Query: map[string]string{"amount": amount},
	})
}

func TestCheckRoute_NoRoute_WhenTheProbeRoutes(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	checker, calls := routeChecker(t, aapl)
	noRouteFor(t, calls, "25000000")
	_, err := checker.CheckRoute(t.Context(), aapl.ID, app.SideBuy, money.NewBaseUnits(25_000_000, 6))
	wantCode(t, err, errs.CodeNoRoute, calls, 2)
	if got := calls.asked().Get("amount"); got != "1000000" {
		t.Fatalf("probe amount = %s, want 1 USDC", got)
	}
	if errs.Message(errs.CodeNoRoute) != "No route for this trade right now. Try a smaller amount." {
		t.Fatalf("message = %q", errs.Message(errs.CodeNoRoute))
	}
}

func TestCheckRoute_AssetPaused_WhenTheProbeDoesNotRoute(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	checker, calls := routeChecker(t, aapl)
	scriptQuote(t, calls.srv, fakes.Step{
		Route: orderRoute, Action: fakes.ActionSucceed, Fixture: orderRoute + "/no-route", Times: 2,
	})
	_, err := checker.CheckRoute(t.Context(), aapl.ID, app.SideBuy, money.NewBaseUnits(25_000_000, 6))
	wantCode(t, err, errs.CodeAssetPaused, calls, 2)
	if errs.KindOf(errs.CodeAssetPaused) != errs.KindBlocked || errs.Message(errs.CodeAssetPaused) !=
		"This stock can't be traded right now." {
		t.Fatalf("asset_paused kind = %v, message = %q",
			errs.KindOf(errs.CodeAssetPaused), errs.Message(errs.CodeAssetPaused))
	}
}

func TestCheckRoute_NoRoute_WithoutAProbeWhenTheAmountIsAtOrBelowIt(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		side   app.Side
		amount money.BaseUnits
	}{
		"buy at the probe":     {app.SideBuy, money.NewBaseUnits(1_000_000, 6)},
		"buy below the probe":  {app.SideBuy, money.NewBaseUnits(250_000, 6)},
		"sell at one token":    {app.SideSell, money.NewBaseUnits(100_000_000, 8)},
		"sell below one token": {app.SideSell, money.NewBaseUnits(5_000_000, 8)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			aapl := marketfake.AAPLx()
			checker, calls := routeChecker(t, aapl)
			scriptQuote(t, calls.srv, fakes.Step{
				Route: orderRoute, Action: fakes.ActionSucceed, Fixture: orderRoute + "/no-route",
			})
			_, err := checker.CheckRoute(t.Context(), aapl.ID, tc.side, tc.amount)
			wantCode(t, err, errs.CodeNoRoute, calls, 1)
		})
	}
}

func TestCheckRoute_SellProbesAtMostOneWholeToken(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	checker, calls := routeChecker(t, aapl)
	noRouteFor(t, calls, "500000000")
	_, err := checker.CheckRoute(t.Context(), aapl.ID, app.SideSell, money.NewBaseUnits(500_000_000, 8))
	wantCode(t, err, errs.CodeNoRoute, calls, 2)
	if got := calls.asked().Get("amount"); got != "100000000" {
		t.Fatalf("probe amount = %s, want one whole AAPLx", got)
	}
}

func TestCheckRoute_AFailedProbeIsNeitherNoRouteNorPaused(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	checker, calls := routeChecker(t, aapl)
	noRouteFor(t, calls, "25000000")
	scriptQuote(t, calls.srv, fakes.Step{
		Route: orderRoute, Action: fakes.ActionFail, Status: http.StatusInternalServerError,
		Query: map[string]string{"amount": "1000000"},
	})
	_, err := checker.CheckRoute(t.Context(), aapl.ID, app.SideBuy, money.NewBaseUnits(25_000_000, 6))
	wantCode(t, err, errs.CodeJupiterUnavailable, calls, 2)
	if !app.ProbeFailed(err) {
		t.Fatal("a failed probe must be marked so preview can advise no_route")
	}
}

func TestCheckRoute_FullQuoteFailureIsNotAProbeFailure(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	checker, calls := routeChecker(t, aapl)
	scriptQuote(t, calls.srv, fakes.Step{
		Route: orderRoute, Action: fakes.ActionFail, Status: http.StatusInternalServerError,
	})
	_, err := checker.CheckRoute(t.Context(), aapl.ID, app.SideBuy, money.NewBaseUnits(25_000_000, 6))
	if app.ProbeFailed(err) {
		t.Fatal("the full quote failing is not a probe failure")
	}
}

func TestCheckRoute_SellProbeIsOneDollarOfTokensAtTheSampledPrice(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	checker, calls := pricedRouteChecker(t, sampledPrice{micros: 230_000_000}, aapl)
	noRouteFor(t, calls, "500000000")
	_, err := checker.CheckRoute(t.Context(), aapl.ID, app.SideSell, money.NewBaseUnits(500_000_000, 8))
	wantCode(t, err, errs.CodeNoRoute, calls, 2)
	if got := calls.asked().Get("amount"); got != "434782" {
		t.Fatalf("probe amount = %s, want $1 of AAPLx at $230", got)
	}
}

func TestCheckRoute_SellProbeAppliesTheUIMultiplier(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		m    domain.Multiplier
		want string
	}{
		"ten shares per token":        {domain.Multiplier{Num: 10, Den: 1}, "43478"},
		"a tenth of a share":          {domain.Multiplier{Num: 1, Den: 10}, "4347826"},
		"an unusable multiplier":      {domain.Multiplier{}, "100000000"},
		"a multiplier that overflows": {domain.Multiplier{Num: 1, Den: math.MaxInt64}, "100000000"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			aapl := marketfake.AAPLx()
			aapl.UIMultiplier = tc.m
			checker, calls := pricedRouteChecker(t, sampledPrice{micros: 230_000_000}, aapl)
			noRouteFor(t, calls, "500000000")
			_, err := checker.CheckRoute(t.Context(), aapl.ID, app.SideSell, money.NewBaseUnits(500_000_000, 8))
			wantCode(t, err, errs.CodeNoRoute, calls, 2)
			if got := calls.asked().Get("amount"); got != tc.want {
				t.Fatalf("probe amount = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestCheckRoute_SellAtOrBelowTheDollarProbeIsNoRouteWithoutAProbe(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	checker, calls := pricedRouteChecker(t, sampledPrice{micros: 230_000_000}, aapl)
	noRouteFor(t, calls, "434782")
	_, err := checker.CheckRoute(t.Context(), aapl.ID, app.SideSell, money.NewBaseUnits(434_782, 8))
	wantCode(t, err, errs.CodeNoRoute, calls, 1)
}

func TestCheckRoute_SellProbeFallsBackToOneTokenWhenThePriceIsUnknown(t *testing.T) {
	t.Parallel()
	for name, prices := range map[string]sampledPrice{
		"no sample":     {},
		"price error":   {err: errs.New(errs.CodeInternal, "test")},
		"price too big": {micros: 1 << 62},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			aapl := marketfake.AAPLx()
			checker, calls := pricedRouteChecker(t, prices, aapl)
			noRouteFor(t, calls, "500000000")
			_, err := checker.CheckRoute(t.Context(), aapl.ID, app.SideSell, money.NewBaseUnits(500_000_000, 8))
			wantCode(t, err, errs.CodeNoRoute, calls, 2)
			if got := calls.asked().Get("amount"); got != "100000000" {
				t.Fatalf("probe amount = %s, want one whole AAPLx", got)
			}
		})
	}
}

func TestCheckRoute_RoutedQuoteMakesNoProbe(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	checker, calls := routeChecker(t, aapl)
	_, err := checker.CheckRoute(t.Context(), aapl.ID, app.SideBuy, money.NewBaseUnits(25_000_000, 6))
	if err != nil || calls.calls.Load() != 1 {
		t.Fatalf("err = %v after %d quotes, want none after the one quote", err, calls.calls.Load())
	}
}

func TestCheckRoute_AssetUntradable(t *testing.T) {
	t.Parallel()
	halted := marketfake.JPSTx()
	checker, calls := routeChecker(t, halted)
	_, err := checker.CheckRoute(t.Context(), halted.ID, app.SideBuy, money.NewBaseUnits(1, 6))
	wantCode(t, err, errs.CodeAssetUntradable, calls, 0)
	if errs.KindOf(errs.CodeAssetUntradable) != errs.KindBlocked || errs.Retryable(errs.CodeAssetUntradable) {
		t.Fatal("asset_untradable must be blocked and not retryable")
	}
	if errs.Message(errs.CodeAssetUntradable) != "This asset can't be traded right now." {
		t.Fatalf("message = %q", errs.Message(errs.CodeAssetUntradable))
	}
}

func TestCheckRoute_OverrideOff(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	aapl.Override = domain.OverrideOff
	checker, calls := routeChecker(t, aapl)
	_, err := checker.CheckRoute(t.Context(), aapl.ID, app.SideSell, money.NewBaseUnits(1, 8))
	wantCode(t, err, errs.CodeAssetUntradable, calls, 0)
}

func TestCheckRoute_AssetNotFound(t *testing.T) {
	t.Parallel()
	checker, calls := routeChecker(t, marketfake.AAPLx())
	missing, err := domain.ParseAssetID("01920000-0000-7000-8000-0000000000ff")
	if err != nil {
		t.Fatal(err)
	}
	_, err = checker.CheckRoute(t.Context(), missing, app.SideBuy, money.NewBaseUnits(1, 6))
	wantCode(t, err, errs.CodeAssetNotFound, calls, 0)
}

func TestCheckRoute_ZeroAmount(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	checker, calls := routeChecker(t, aapl)
	_, err := checker.CheckRoute(t.Context(), aapl.ID, app.SideBuy, money.NewBaseUnits(0, 6))
	wantCode(t, err, errs.CodeInvalidInput, calls, 0)
}

func TestCheckRoute_WrongDecimals(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	checker, calls := routeChecker(t, aapl)
	_, err := checker.CheckRoute(t.Context(), aapl.ID, app.SideSell, money.NewBaseUnits(1_000_000, 6))
	wantCode(t, err, errs.CodeInvalidInput, calls, 0)
}

func TestCheckRoute_BadSide(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	checker, calls := routeChecker(t, aapl)
	_, err := checker.CheckRoute(t.Context(), aapl.ID, app.Side("hold"), money.NewBaseUnits(1, 6))
	wantCode(t, err, errs.CodeInvalidInput, calls, 0)
}

func TestCheckRoute_JupiterUnavailable(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	checker, calls := routeChecker(t, aapl)
	scriptQuote(t, calls.srv, fakes.Step{
		Route: orderRoute, Action: fakes.ActionFail, Status: http.StatusInternalServerError,
	})
	_, err := checker.CheckRoute(t.Context(), aapl.ID, app.SideBuy, money.NewBaseUnits(25_000_000, 6))
	var got *errs.Error
	if !errors.As(err, &got) || got.Code != errs.CodeJupiterUnavailable || got.Op != "jupiter.Quote" {
		t.Fatalf("err = %#v, want jupiter.Quote's jupiter_unavailable unchanged", err)
	}
	if calls.calls.Load() != 1 {
		t.Fatalf("calls = %d, want the one quote", calls.calls.Load())
	}
}
