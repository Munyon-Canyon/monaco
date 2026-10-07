package fakes

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Trading struct {
	mu    sync.Mutex
	swaps []trading.SwapView
	err   error
	once  error
	clock clock.Clock
}

var _ trading.Queries = (*Trading)(nil)

func NewTrading(swaps ...trading.SwapView) *Trading {
	f := &Trading{clock: clock.Real{}}
	for _, s := range swaps {
		f.Put(s)
	}
	return f
}

func (f *Trading) At(c clock.Clock) *Trading {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clock = c
	return f
}

func (f *Trading) Put(s trading.SwapView) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i := slices.IndexFunc(f.swaps, func(v trading.SwapView) bool { return v.ID == s.ID }); i >= 0 {
		f.swaps[i] = s
		return
	}
	f.swaps = append(f.swaps, s)
}

func (f *Trading) Fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *Trading) FailOnce(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.once = err
}

func (f *Trading) failure() error {
	if err := f.once; err != nil {
		f.once = nil
		return err
	}
	return f.err
}

func (f *Trading) find(op string, match func(trading.SwapView) bool) (trading.SwapView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failure(); err != nil {
		return trading.SwapView{}, err
	}
	if i := slices.IndexFunc(f.swaps, match); i >= 0 {
		return f.swaps[i], nil
	}
	return trading.SwapView{}, errs.New(errs.CodeSwapNotFound, op)
}

func (f *Trading) Swap(_ context.Context, id ids.SwapID) (trading.SwapView, error) {
	return f.find("fakes.Trading.Swap", func(v trading.SwapView) bool { return v.ID == id })
}

func (f *Trading) SwapBySignature(_ context.Context, sig chain.Signature) (trading.SwapView, error) {
	return f.find("fakes.Trading.SwapBySignature", func(v trading.SwapView) bool {
		return sig != "" && v.TxSignature == sig
	})
}

func (f *Trading) LatestBySource(_ context.Context, src trading.Source) (trading.SwapView, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failure(); err != nil {
		return trading.SwapView{}, false, err
	}
	var latest trading.SwapView
	found := false
	for _, v := range f.swaps {
		if v.Source == src && (!found || !v.CreatedAt.Before(latest.CreatedAt)) {
			latest, found = v, true
		}
	}
	return latest, found, nil
}

func (f *Trading) HasLiveSwap(_ context.Context, src trading.Source) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failure(); err != nil {
		return false, err
	}
	return slices.ContainsFunc(f.swaps, func(v trading.SwapView) bool {
		return v.Source == src && v.Status != domain.StatusFailed
	}), nil
}

func (f *Trading) OwnsSignature(ctx context.Context, sig chain.Signature) (bool, error) {
	_, err := f.SwapBySignature(ctx, sig)
	if errs.CodeOf(err) == errs.CodeSwapNotFound {
		return false, nil
	}
	return err == nil, err
}

func (f *Trading) stuck(olderThan time.Duration) ([]trading.SwapView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failure(); err != nil {
		return nil, err
	}
	cutoff := f.clock.Now().Add(-olderThan)
	var out []trading.SwapView
	for _, v := range f.swaps {
		unfinished := v.Status == domain.StatusCreated || v.Status == domain.StatusSubmitted
		if unfinished && v.CreatedAt.Before(cutoff) {
			out = append(out, v)
		}
	}
	slices.SortFunc(out, func(a, b trading.SwapView) int { return a.CreatedAt.Compare(b.CreatedAt) })
	return out, nil
}

func (f *Trading) Stuck(_ context.Context, olderThan time.Duration, limit int) ([]trading.SwapView, error) {
	out, err := f.stuck(olderThan)
	return out[:min(limit, len(out))], err
}

func (f *Trading) CountStuck(_ context.Context, olderThan time.Duration) (int, error) {
	out, err := f.stuck(olderThan)
	return len(out), err
}

func (f *Trading) ExecuteRequestID(_ context.Context, id ids.SwapID) (string, error) {
	match := func(v trading.SwapView) bool { return v.ID == id }
	if _, err := f.find("fakes.Trading.ExecuteRequestID", match); err != nil {
		return "", err
	}
	return "request-" + id.UUID().String(), nil
}
