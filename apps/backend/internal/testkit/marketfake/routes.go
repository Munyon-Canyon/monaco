package marketfake

import (
	"context"
	"log/slog"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

var _ market.Routes = (*RoutesFake)(nil)

type scriptedRoute struct {
	ok    bool
	code  errs.Code
	check market.RouteCheck
}

type RoutesFake struct {
	testkit.Faults

	mu sync.Mutex
	by map[market.AssetID]scriptedRoute
}

func (f *RoutesFake) Ok(id market.AssetID, check market.RouteCheck) {
	f.put(id, scriptedRoute{ok: true, check: check})
}

func (f *RoutesFake) NoRoute(id market.AssetID) {
	f.put(id, scriptedRoute{code: errs.CodeNoRoute})
}

func (f *RoutesFake) Untradable(id market.AssetID) {
	f.put(id, scriptedRoute{code: errs.CodeAssetUntradable})
}

func (f *RoutesFake) put(id market.AssetID, script scriptedRoute) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.by == nil {
		f.by = map[market.AssetID]scriptedRoute{}
	}
	f.by[id] = script
}

func (f *RoutesFake) CheckRoute(
	_ context.Context, id market.AssetID, _ market.Side, _ money.BaseUnits,
) (market.RouteCheck, error) {
	if err := f.Check("CheckRoute"); err != nil {
		return market.RouteCheck{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	script, found := f.by[id]
	if !found {
		return market.RouteCheck{}, errs.New(
			errs.CodeAssetNotFound, "marketfake.CheckRoute", slog.String("id", id.String()))
	}
	if !script.ok {
		return market.RouteCheck{}, errs.New(script.code, "marketfake.CheckRoute", slog.String("id", id.String()))
	}
	return script.check, nil
}
