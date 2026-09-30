package marketfake

import (
	"context"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

var _ market.Prices = (*PricesFake)(nil)

type PricesFake struct {
	testkit.Faults

	mu      sync.Mutex
	samples map[market.AssetID][]market.Price
}

func (f *PricesFake) Set(asset market.AssetID, micros money.Micros, observedAt time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.samples == nil {
		f.samples = map[market.AssetID][]market.Price{}
	}
	f.samples[asset] = append(f.samples[asset], market.Price{Micros: micros, ObservedAt: observedAt})
}

func (f *PricesFake) LatestPrices(context.Context) (map[market.AssetID]market.Price, error) {
	if err := f.Check("LatestPrices"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[market.AssetID]market.Price{}
	for id, samples := range f.samples {
		if p, ok := newest(samples, func(market.Price) bool { return true }); ok {
			out[id] = p
		}
	}
	return out, nil
}

func (f *PricesFake) PricesAsOf(
	_ context.Context, ids []market.AssetID, at time.Time,
) (map[market.AssetID]market.Price, error) {
	if err := f.Check("PricesAsOf"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[market.AssetID]market.Price{}
	for _, id := range ids {
		if p, ok := newest(f.samples[id], func(p market.Price) bool { return !p.ObservedAt.After(at) }); ok {
			out[id] = p
		}
	}
	return out, nil
}

func newest(samples []market.Price, keep func(market.Price) bool) (market.Price, bool) {
	var best market.Price
	found := false
	for _, p := range samples {
		if keep(p) && (!found || p.ObservedAt.After(best.ObservedAt)) {
			best, found = p, true
		}
	}
	return best, found
}
