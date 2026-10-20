package marketfake

import (
	"context"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

var (
	_ app.PriceHistory = (*PriceHistoryFake)(nil)
	_ app.VolumeSource = (*PriceHistoryFake)(nil)
)

type HistoryCall struct {
	Mint domain.Mint
	Days int
}

type PriceHistoryFake struct {
	testkit.Faults

	mu      sync.Mutex
	keyless bool
	charts  map[HistoryCall][]app.Sample
	volumes map[HistoryCall]app.Volume
	calls   []HistoryCall
	spot    map[domain.Mint]money.Micros
	batches [][]domain.Mint
	during  func()
}

func (f *PriceHistoryFake) WithoutKey() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keyless = true
}

func (f *PriceHistoryFake) Put(mint domain.Mint, days int, samples ...app.Sample) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.charts == nil {
		f.charts = map[HistoryCall][]app.Sample{}
	}
	f.charts[HistoryCall{Mint: mint, Days: days}] = samples
}

func (f *PriceHistoryFake) PutVolume(mint domain.Mint, days int, volume app.Volume) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.volumes == nil {
		f.volumes = map[HistoryCall]app.Volume{}
	}
	f.volumes[HistoryCall{Mint: mint, Days: days}] = volume
}

func (f *PriceHistoryFake) ClearVolume(mint domain.Mint, days int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.volumes, HistoryCall{Mint: mint, Days: days})
}

func (f *PriceHistoryFake) During(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.during = fn
}

func (f *PriceHistoryFake) Calls() []HistoryCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]HistoryCall(nil), f.calls...)
}

func (f *PriceHistoryFake) Configured() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.keyless
}

func (f *PriceHistoryFake) MarketChart(_ context.Context, mint domain.Mint, days int) (app.History, error) {
	call := HistoryCall{Mint: mint, Days: days}
	f.mu.Lock()
	f.calls = append(f.calls, call)
	during := f.during
	f.mu.Unlock()
	if during != nil {
		during()
	}
	if err := f.Check("MarketChart"); err != nil {
		return app.History{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := app.History{Prices: append([]app.Sample(nil), f.charts[call]...)}
	if v, ok := f.volumes[call]; ok {
		out.Volume24h = &v
	}
	return out, nil
}

func (f *PriceHistoryFake) PutSpotVolume(mint domain.Mint, usd money.Micros) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.spot == nil {
		f.spot = map[domain.Mint]money.Micros{}
	}
	f.spot[mint] = usd
}

func (f *PriceHistoryFake) VolumeBatches() [][]domain.Mint {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]domain.Mint(nil), f.batches...)
}

func (f *PriceHistoryFake) Volumes(_ context.Context, mints []domain.Mint) (map[domain.Mint]money.Micros, error) {
	f.mu.Lock()
	f.batches = append(f.batches, append([]domain.Mint(nil), mints...))
	f.mu.Unlock()
	if err := f.Check("Volumes"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[domain.Mint]money.Micros{}
	for _, m := range mints {
		if v, ok := f.spot[m]; ok {
			out[m] = v
		}
	}
	return out, nil
}
