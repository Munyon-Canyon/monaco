package app

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func (r RunValuation) listingPrices(
	ctx context.Context, positions []treasury.CabalPositions, at time.Time,
) (map[market.ListingKey]market.ListingPrice, error) {
	var keys []market.ListingKey
	for _, p := range positions {
		for _, l := range p.Listings {
			keys = append(keys, market.ListingKey(l.Key))
		}
	}
	if len(keys) == 0 {
		return map[market.ListingKey]market.ListingPrice{}, nil
	}
	return r.ports.Market.ListingPrices(ctx, keys, at)
}

type unpricedListing struct {
	key    string
	reason string
}

func offSolana(
	in valuationInput, position treasury.CabalPositions, prices map[market.ListingKey]market.ListingPrice,
	at time.Time,
) (valuationInput, *unpricedListing) {
	if in.unpriced {
		return in, nil
	}
	total, err := in.cash.Add(position.EVMCash)
	if err == nil {
		total, err = total.Add(position.InTransit)
	}
	if err != nil {
		return unpricedInput(in), &unpricedListing{reason: "overflow"}
	}
	for _, l := range position.Listings {
		if reason := listingPriceGap(prices, l.Key, at); reason != "" {
			return unpricedInput(in), &unpricedListing{key: l.Key, reason: reason}
		}
		value, err := prices[market.ListingKey(l.Key)].Value(l.Units)
		if err == nil {
			total, err = total.Add(value)
		}
		if err != nil {
			return unpricedInput(in), &unpricedListing{key: l.Key, reason: "unvalued"}
		}
	}
	in.cash = total
	return in, nil
}

func listingPriceGap(prices map[market.ListingKey]market.ListingPrice, key string, at time.Time) string {
	price, ok := prices[market.ListingKey(key)]
	switch {
	case !ok:
		return "no_listing"
	case price.Price == nil:
		return "no_price"
	case at.Sub(price.Price.ObservedAt) > multichain.ListingPriceMaxAge:
		return "stale"
	}
	return ""
}

const unpricedLogEvery = 10 * time.Minute

type unpricedLog struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func (l *unpricedLog) log(ctx context.Context, cabal ids.CabalID, miss *unpricedListing, at time.Time) {
	if miss == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	key := cabal.String() + "|" + miss.key + "|" + miss.reason
	if last, seen := l.last[key]; seen && at.Sub(last) < unpricedLogEvery {
		return
	}
	l.last[key] = at
	observability.Degraded(ctx, observability.RankingCabalUnpricedListing, slog.String("cabal", cabal.String()),
		slog.String("listing", miss.key), slog.String("reason", miss.reason))
}

func unpricedInput(in valuationInput) valuationInput {
	return valuationInput{cabalID: in.cabalID, position: in.position, reserved: in.reserved, unpriced: true}
}
