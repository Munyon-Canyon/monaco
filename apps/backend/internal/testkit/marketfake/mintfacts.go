package marketfake

import (
	"context"
	"log/slog"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

var _ app.MintFacts = (*MintFacts)(nil)

type MintFacts struct {
	testkit.Faults

	mu    sync.Mutex
	facts map[market.Mint]mintFact
	asked int
}

type mintFact struct {
	decimals uint8
	num, den uint64
}

func (f *MintFacts) Put(mint market.Mint, decimals uint8, multiplierNum, multiplierDen uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.facts == nil {
		f.facts = map[market.Mint]mintFact{}
	}
	f.facts[mint] = mintFact{decimals: decimals, num: multiplierNum, den: multiplierDen}
}

func (f *MintFacts) Facts(
	_ context.Context, mint market.Mint,
) (decimals uint8, multiplierNum, multiplierDen uint64, err error) {
	f.mu.Lock()
	f.asked++
	f.mu.Unlock()
	if err = f.Check("Facts"); err != nil {
		return 0, 0, 0, err
	}
	if err = f.Check("Facts:" + mint.String()); err != nil {
		return 0, 0, 0, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	got, ok := f.facts[mint]
	if !ok {
		return 0, 0, 0, errs.New(errs.CodeNotFound, "marketfake.MintFacts.Facts", slog.String("mint", mint.String()))
	}
	return got.decimals, got.num, got.den, nil
}

func (f *MintFacts) Asked() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.asked
}
