package marketfake_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestRoutesFake_scriptsOkNoRouteAndUntradable(t *testing.T) {
	t.Parallel()
	aapl, tsla, jpst := marketfake.AAPLx().ID, marketfake.TSLAx().ID, marketfake.JPSTx().ID
	missing, err := domain.ParseAssetID("01920000-0000-7000-8000-0000000000ff")
	if err != nil {
		t.Fatal(err)
	}
	want := market.RouteCheck{
		InAmount: money.NewBaseUnits(25_000_000, 6), OutAmount: money.NewBaseUnits(11_000_000, 8),
		PriceImpactBps: 12, QuotedAt: time.Date(2026, 10, 2, 15, 0, 0, 0, time.UTC),
	}
	var f marketfake.RoutesFake
	f.Ok(aapl, want)
	f.NoRoute(tsla)
	f.Untradable(jpst)
	got, err := f.CheckRoute(t.Context(), aapl, market.SideBuy, money.NewBaseUnits(1, 6))
	if err != nil || got != want {
		t.Fatalf("Ok = %+v, %v, want the scripted check", got, err)
	}
	_, err = f.CheckRoute(t.Context(), tsla, market.SideSell, money.NewBaseUnits(1, 8))
	if errs.CodeOf(err) != errs.CodeNoRoute {
		t.Fatalf("NoRoute err = %v, want no_route", err)
	}
	_, err = f.CheckRoute(t.Context(), jpst, market.SideBuy, money.NewBaseUnits(1, 6))
	if errs.CodeOf(err) != errs.CodeAssetUntradable {
		t.Fatalf("Untradable err = %v, want asset_untradable", err)
	}
	_, err = f.CheckRoute(t.Context(), missing, market.SideBuy, money.NewBaseUnits(1, 6))
	if errs.CodeOf(err) != errs.CodeAssetNotFound {
		t.Fatalf("missing err = %v, want asset_not_found", err)
	}
}

func TestRoutesFake_failsWhereFaultsSay(t *testing.T) {
	t.Parallel()
	var f marketfake.RoutesFake
	f.Ok(marketfake.AAPLx().ID, market.RouteCheck{})
	f.FailOnce("CheckRoute", errs.New(errs.CodeDBUnavailable, "test"))
	_, err := f.CheckRoute(t.Context(), marketfake.AAPLx().ID, market.SideBuy, money.NewBaseUnits(1, 6))
	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("CheckRoute err = %v, want the scripted fault", err)
	}
	if _, err = f.CheckRoute(t.Context(), marketfake.AAPLx().ID, market.SideBuy, money.NewBaseUnits(1, 6)); err != nil {
		t.Fatalf("CheckRoute after the fault = %v, want the scripted check", err)
	}
}
