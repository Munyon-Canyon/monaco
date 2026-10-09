package app

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type HotMints func(context.Context) ([]chain.SolanaAddress, error)

type priceNeeds struct {
	assets []domain.Asset
	hot    int
}

func (n priceNeeds) mints() []domain.Mint {
	out := make([]domain.Mint, len(n.assets))
	for i, a := range n.assets {
		out[i] = a.Mint
	}
	return out
}

func (p *SamplePrices) needs(ctx context.Context, all []domain.Asset, limit int) (priceNeeds, error) {
	hot, err := hotSet(ctx, p.hot, all)
	if err != nil {
		return priceNeeds{}, errs.Wrap(err, errs.CodeOf(err), "market.SamplePrices.needs")
	}
	var n priceNeeds
	var rest []domain.Asset
	for _, a := range all {
		switch {
		case hot[a.Mint.Address()]:
			n.assets = append(n.assets, a)
		case a.Tradable():
			rest = append(rest, a)
		}
	}
	n.hot = len(n.assets)
	restAll := rest
	if room := limit - n.hot; room < len(rest) {
		rest = nil
		if room > 0 {
			rest = coldSlot(restAll, p.clock.Now(), p.interval, room)
		}
	}
	n.assets = append(n.assets, rest...)
	return n, nil
}

func hotSet(ctx context.Context, readers []HotMints, all []domain.Asset) (map[chain.SolanaAddress]bool, error) {
	hot := map[chain.SolanaAddress]bool{}
	for _, read := range readers {
		mints, err := read(ctx)
		if err != nil {
			return nil, err
		}
		for _, m := range mints {
			hot[m] = true
		}
	}
	for _, a := range all {
		if a.PopularRank > 0 {
			hot[a.Mint.Address()] = true
		}
	}
	return hot, nil
}

func coldSlot(assets []domain.Asset, now time.Time, interval time.Duration, size int) []domain.Asset {
	if len(assets) == 0 {
		return nil
	}
	byMint := slices.SortedFunc(slices.Values(assets), func(a, b domain.Asset) int {
		return strings.Compare(a.Mint.String(), b.Mint.String())
	})
	slots := (len(byMint) + size - 1) / size
	slot := int(now.UnixNano()/int64(interval)) % slots
	return byMint[slot*size : min((slot+1)*size, len(byMint))]
}
