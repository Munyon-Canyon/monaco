package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

type MarketListings interface {
	ListingOn(
		ctx context.Context, stock multichain.StockID, n chain.Network,
	) (multichain.ListingID, multichain.Venue, error)
}

type PinListings struct{ Market MarketListings }

var _ app.PinListings = PinListings{}

func (p PinListings) PinListing(
	ctx context.Context, stock multichain.StockID, n chain.Network,
) (multichain.ListingID, multichain.Venue, bool, error) {
	id, venue, err := p.Market.ListingOn(ctx, stock, n)
	switch {
	case errs.CodeOf(err) == errs.CodeAssetNotFound:
		return multichain.ListingID{}, "", false, nil
	case err != nil:
		return multichain.ListingID{}, "", false, err
	}
	return id, venue, true, nil
}
