package app

import (
	"cmp"
	"context"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const (
	starterDefault = 5
	starterFloor   = 3
	starterTTL     = 5 * time.Minute
)

type HolderCount struct {
	Mint   chain.SolanaAddress
	Cabals int
}

type HolderCounts func(context.Context) ([]HolderCount, error)

type holderBook struct {
	mu     sync.Mutex
	loaded bool
	at     time.Time
	counts []HolderCount
}

func (l *ListAssets) WithHolders(counts HolderCounts) *ListAssets {
	l.holders = counts
	return l
}

func (l *ListAssets) starters(ctx context.Context, q listQuery, now time.Time) ([]Summary, error) {
	const op = "market.ListAssets.starters"
	all, err := l.catalog.ListAll(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	counts, err := l.heldCounts(ctx, now)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	picked, counted := pickStarters(all, counts, q)
	items, err := l.summaries(ctx, picked, now)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if n, ok := counted[items[i].Asset.Mint.Address()]; ok {
			items[i].CabalCount = &n
		}
	}
	return items, nil
}

func (l *ListAssets) heldCounts(ctx context.Context, now time.Time) ([]HolderCount, error) {
	if l.holders == nil {
		return nil, nil
	}
	b := &l.held
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.loaded && now.Sub(b.at) < starterTTL {
		return b.counts, nil
	}
	counts, err := l.holders(ctx)
	if err != nil {
		return nil, err
	}
	b.loaded, b.at, b.counts = true, now, counts
	return counts, nil
}

func pickStarters(all []domain.Asset, counts []HolderCount, q listQuery) ([]domain.Asset, map[chain.SolanaAddress]int) {
	listed := map[chain.SolanaAddress]domain.Asset{}
	var popular []domain.Asset
	for _, a := range all {
		if !a.Tradable() || !matches(a, q.q) {
			continue
		}
		listed[a.Mint.Address()] = a
		if a.PopularRank > 0 {
			popular = append(popular, a)
		}
	}
	counted := map[chain.SolanaAddress]int{}
	var ranked []domain.Asset
	for _, c := range counts {
		a, ok := listed[c.Mint]
		if _, dup := counted[c.Mint]; !ok || dup || c.Cabals < starterFloor {
			continue
		}
		counted[c.Mint] = c.Cabals
		ranked = append(ranked, a)
	}
	slices.SortFunc(ranked, func(a, b domain.Asset) int {
		return cmp.Or(
			counted[b.Mint.Address()]-counted[a.Mint.Address()],
			cmp.Compare(rankOrLast(a), rankOrLast(b)),
			strings.Compare(a.Symbol, b.Symbol),
		)
	})
	slices.SortFunc(popular, func(a, b domain.Asset) int {
		return cmp.Or(cmp.Compare(a.PopularRank, b.PopularRank), strings.Compare(a.Symbol, b.Symbol))
	})
	for _, a := range popular {
		if _, held := counted[a.Mint.Address()]; !held {
			ranked = append(ranked, a)
		}
	}
	return ranked[:min(len(ranked), int(q.limit))], counted
}

func rankOrLast(a domain.Asset) int {
	if a.PopularRank <= 0 {
		return math.MaxInt
	}
	return int(a.PopularRank)
}
