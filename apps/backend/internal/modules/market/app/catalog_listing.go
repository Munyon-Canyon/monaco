package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"math/big"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

var _ port.Listings = (*Catalog)(nil)

func (c *Catalog) Listing(ctx context.Context, id multichain.ListingID) (multichain.Listing, error) {
	const op = "market.Catalog.Listing"
	key := slog.String("listing_id", id.String())
	row, err := c.q.FindListing(ctx, id.UUID())
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
		ID: id, StockID: stock, Issuer: row.Issuer, Symbol: row.Symbol, Network: chain.Network(row.Network),
		TokenAddress: row.TokenAddress, WrapperAddress: row.WrapperAddress.String, Decimals: decimals,
		SharesPerToken: shares, Venue: multichain.Venue(row.Venue),
	}, nil
}
