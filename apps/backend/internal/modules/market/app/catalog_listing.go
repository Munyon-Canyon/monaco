package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"math/big"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

var _ port.Listings = (*Catalog)(nil)

func (c *Catalog) Listing(ctx context.Context, id multichain.ListingID) (multichain.Listing, error) {
	row, err := c.q.FindListing(ctx, id.UUID())
	return listingFromRow(row, err, "market.Catalog.Listing", slog.String("listing_id", id.String()))
}

func (c *Catalog) ListingByAsset(ctx context.Context, asset string) (multichain.Listing, error) {
	const op = "market.Catalog.ListingByAsset"
	key := slog.String("asset", asset)
	mint, err := domain.ParseMint(asset)
	if err != nil {
		return multichain.Listing{}, errs.Wrap(err, errs.CodeOf(err), op, key)
	}
	network, token, listed := mint.Listing()
	if !listed {
		network, token = chain.NetworkSolana, mint.String()
	}
	row, err := c.q.FindListingByToken(
		ctx,
		sqlc.FindListingByTokenParams{Network: string(network), TokenAddress: token},
	)
	return listingFromRow(sqlc.FindListingRow(row), err, op, key)
}

func (c *Catalog) ListingOn(
	ctx context.Context, stock multichain.StockID, n chain.Network,
) (multichain.ListingID, multichain.Venue, error) {
	const op = "market.Catalog.ListingOn"
	key := slog.String("network", string(n))
	row, err := c.q.FindListingForAssetOn(ctx, sqlc.FindListingForAssetOnParams{
		AssetID: stock.UUID(), Network: string(n),
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return multichain.ListingID{}, "", errs.New(errs.CodeAssetNotFound, op, key)
	case err != nil:
		return multichain.ListingID{}, "", errs.Wrap(err, errs.CodeInternal, op, key)
	}
	return multichain.ListingIDFrom(row.ID), multichain.Venue(row.Venue), nil
}

func listingFromRow(row sqlc.FindListingRow, err error, op string, key slog.Attr) (multichain.Listing, error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return multichain.Listing{}, errs.New(errs.CodeAssetNotFound, op, key)
	case err != nil:
		return multichain.Listing{}, errs.Wrap(err, errs.CodeInternal, op, key)
	case !row.Decimals.Valid || !row.VerifiedAt.Valid:
		return multichain.Listing{}, errs.New(errs.CodeAssetUntradable, op, key)
	}
	shares, _ := new(big.Rat).SetString(row.SharesPerToken)
	decimals, decimalsErr := decimalsOf(row.Decimals.Int16)
	var stock multichain.StockID
	var stockErr error
	if row.AssetID.Valid {
		stockErr = stock.UnmarshalText([]byte(uuid.UUID(row.AssetID.Bytes).String()))
	}
	if err := errors.Join(decimalsErr, stockErr); err != nil {
		return multichain.Listing{}, errs.Wrap(err, errs.CodeDecodeFailed, op, key)
	}
	return multichain.Listing{
		ID: multichain.ListingIDFrom(row.ID), StockID: stock, Issuer: row.Issuer, Symbol: row.Symbol,
		Network: chain.Network(row.Network), TokenAddress: row.TokenAddress, WrapperAddress: row.WrapperAddress.String,
		Decimals: decimals, SharesPerToken: shares, Venue: multichain.Venue(row.Venue), Enabled: row.Enabled,
	}, nil
}
