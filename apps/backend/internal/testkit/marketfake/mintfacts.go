package marketfake

import (
	"context"
	"log/slog"
	"sync"
	"time"

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
	decimals         uint8
	num, den         uint64
	nextNum, nextDen uint64
	nextAt           time.Time
}

func (f *MintFacts) Put(mint market.Mint, decimals uint8, multiplierNum, multiplierDen uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.facts == nil {
		f.facts = map[market.Mint]mintFact{}
	}
	f.facts[mint] = mintFact{decimals: decimals, num: multiplierNum, den: multiplierDen}
}

func (f *MintFacts) Schedule(mint market.Mint, nextNum, nextDen uint64, at time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fact := f.facts[mint]
	fact.nextNum, fact.nextDen, fact.nextAt = nextNum, nextDen, at
	f.facts[mint] = fact
}

func (f *MintFacts) Facts(
	_ context.Context, mints []market.Mint,
) (map[market.Mint]app.MintFact, map[market.Mint]error, error) {
	f.mu.Lock()
	f.asked += len(mints)
	f.mu.Unlock()
	if err := f.Check("Facts"); err != nil {
		return nil, nil, err
	}
	answers := make(map[market.Mint]app.MintFact, len(mints))
	failures := make(map[market.Mint]error)
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, mint := range mints {
		if err := f.Check("Facts:" + mint.String()); err != nil {
			failures[mint] = err
			continue
		}
		got, ok := f.facts[mint]
		if !ok {
			failures[mint] = errs.New(
				errs.CodeNotFound, "marketfake.MintFacts.Facts", slog.String("mint", mint.String()),
			)
			continue
		}
		answers[mint] = app.MintFact{
			Decimals: got.decimals, MultiplierNum: got.num, MultiplierDen: got.den,
			NextMultiplierNum: got.nextNum, NextMultiplierDen: got.nextDen, NextMultiplierAt: got.nextAt,
		}
	}
	return answers, failures, nil
}

func (f *MintFacts) Asked() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.asked
}
