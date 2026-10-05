package marketfake

import (
	"context"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

var _ app.PriceHistory = (*PriceHistoryFake)(nil)

type HistoryCall struct {
	Mint domain.Mint
	Days int
}

type PriceHistoryFake struct {
	testkit.Faults

	mu      sync.Mutex
	keyless bool
	charts  map[HistoryCall][]app.Sample
	calls   []HistoryCall
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

func (f *PriceHistoryFake) MarketChart(_ context.Context, mint domain.Mint, days int) ([]app.Sample, error) {
	call := HistoryCall{Mint: mint, Days: days}
	f.mu.Lock()
	f.calls = append(f.calls, call)
	during := f.during
	f.mu.Unlock()
	if during != nil {
		during()
	}
	if err := f.Check("MarketChart"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]app.Sample(nil), f.charts[call]...), nil
}
