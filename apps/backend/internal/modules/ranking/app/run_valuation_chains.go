package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
)

const listingPriceMaxAge = 5 * time.Minute

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

func offSolana(
	in valuationInput, position treasury.CabalPositions, prices map[market.ListingKey]market.ListingPrice,
	at time.Time,
) valuationInput {
	if in.unpriced {
		return in
	}
	total, err := in.cash.Add(position.EVMCash)
	if err == nil {
		total, err = total.Add(position.InTransit)
	}
	if err != nil {
		return unpricedInput(in)
	}
	for _, l := range position.Listings {
		price, ok := prices[market.ListingKey(l.Key)]
		if !ok || price.Price == nil || at.Sub(price.Price.ObservedAt) > listingPriceMaxAge {
			return unpricedInput(in)
		}
		value, err := price.Value(l.Units)
		if err == nil {
			total, err = total.Add(value)
		}
		if err != nil {
			return unpricedInput(in)
		}
	}
	in.cash = total
	return in
}

func unpricedInput(in valuationInput) valuationInput {
	return valuationInput{cabalID: in.cabalID, position: in.position, reserved: in.reserved, unpriced: true}
}
