package market_test

import (
	"bytes"
	"encoding/json"
	"errors"
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

func routeChecker(t *testing.T, assets ...market.Asset) (*app.RouteChecker, *quoteCalls) {
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
		marketfake.NewCatalog(assets...), jupiterquote.New(client), testkit.NewClock(quotedAt()),
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

func TestCheckRoute_NoRoute(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	checker, calls := routeChecker(t, aapl)
	scriptQuote(t, calls.srv, fakes.Step{
		Route: orderRoute, Action: fakes.ActionSucceed, Fixture: orderRoute + "/no-route",
	})
	_, err := checker.CheckRoute(t.Context(), aapl.ID, app.SideBuy, money.NewBaseUnits(25_000_000, 6))
	wantCode(t, err, errs.CodeNoRoute, calls, 1)
	if errs.Message(errs.CodeNoRoute) != "No route for this trade right now. Try a smaller amount." {
		t.Fatalf("message = %q", errs.Message(errs.CodeNoRoute))
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
	if errs.Message(errs.CodeAssetUntradable) != "This asset can't be traded right now" {
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
