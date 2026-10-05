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

const coldPerTick = 100

type priceNeeds struct {
	mints     []domain.Mint
	hot, cold int
}

func (p *SamplePrices) needs(ctx context.Context, assets []domain.Asset) (priceNeeds, error) {
	hot := map[chain.SolanaAddress]bool{}
	for _, read := range p.hot {
		mints, err := read(ctx)
		if err != nil {
			return priceNeeds{}, errs.Wrap(err, errs.CodeOf(err), "market.SamplePrices.needs")
		}
		for _, m := range mints {
			hot[m] = true
		}
	}
	var n priceNeeds
	for _, a := range assets {
		if hot[a.Mint.Address()] {
			n.mints = append(n.mints, a.Mint)
		}
	}
	n.hot = len(n.mints)
	for _, a := range coldSlot(assets, p.clock.Now(), p.interval) {
		if !hot[a.Mint.Address()] {
			n.mints = append(n.mints, a.Mint)
			n.cold++
		}
	}
	return n, nil
}

func coldSlot(assets []domain.Asset, now time.Time, interval time.Duration) []domain.Asset {
	if len(assets) == 0 {
		return nil
	}
	byMint := slices.SortedFunc(slices.Values(assets), func(a, b domain.Asset) int {
		return strings.Compare(a.Mint.String(), b.Mint.String())
	})
	slots := (len(byMint) + coldPerTick - 1) / coldPerTick
	slot := int(now.UnixNano()/int64(interval)) % slots
	return byMint[slot*coldPerTick : min((slot+1)*coldPerTick, len(byMint))]
}
