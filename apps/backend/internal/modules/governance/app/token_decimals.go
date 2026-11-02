package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

type EVMListings interface {
	ListingOn(
		ctx context.Context, stock multichain.StockID, n chain.Network,
	) (multichain.ListingID, multichain.Venue, error)
	Listing(ctx context.Context, id multichain.ListingID) (multichain.Listing, error)
}

func (r *ProposalReads) Listed(l EVMListings) *ProposalReads {
	r.listings = l
	return r
}

func (r *ProposalReads) tokenDecimals(
	ctx context.Context, asset market.Asset, network chain.Network,
) (uint8, error) {
	const op = "governance.tokenDecimals"
	if !network.IsEVM() || r.listings == nil {
		return asset.Decimals, nil
	}
	id, _, err := r.listings.ListingOn(ctx, multichain.StockID(asset.ID), network)
	if err == nil {
		var listing multichain.Listing
		if listing, err = r.listings.Listing(ctx, id); err == nil {
			return listing.Decimals, nil
		}
	}
	if code := errs.CodeOf(err); code == errs.CodeAssetNotFound || code == errs.CodeAssetUntradable {
		return asset.Decimals, nil
	}
	return 0, errs.Wrap(err, errs.CodeOf(err), op)
}

func (r *ProposalReads) SellDecimals(ctx context.Context, symbol string, network chain.Network) (uint8, error) {
	if !network.IsEVM() {
		return 0, nil
	}
	asset, err := r.assets.AssetBySymbol(ctx, symbol)
	if err != nil {
		return 0, err
	}
	return r.tokenDecimals(ctx, asset, network)
}
