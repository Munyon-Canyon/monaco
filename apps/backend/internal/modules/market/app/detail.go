package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
)

type Listing struct {
	Symbol      string
	DisplayName string
	Issuer      domain.Issuer
	Kind        domain.Kind
	LogoURL     string
	Tradable    bool
}

type AssetView struct {
	Summary Summary
	Others  []Listing
}

type Detailer interface {
	Handle(context.Context, string) (AssetView, error)
}

type detailReader interface {
	AssetBySymbol(context.Context, string) (sqlc.Asset, error)
	OtherListings(context.Context, sqlc.OtherListingsParams) ([]sqlc.OtherListingsRow, error)
}

var _ Detailer = (*Detail)(nil)

var _ detailReader = (*sqlc.Queries)(nil)

type Detail struct {
	list *ListAssets
	read detailReader
}

func NewDetail(db sqlc.DBTX, list *ListAssets) *Detail {
	return &Detail{list: list, read: sqlc.New(db)}
}

func (d *Detail) Handle(ctx context.Context, symbol string) (AssetView, error) {
	row, err := d.read.AssetBySymbol(ctx, symbol)
	asset, err := one(row, err, "market.AssetBySymbol", slog.String("symbol", symbol))
	if err != nil {
		return AssetView{}, err
	}
	sums, err := d.list.summaries(ctx, []domain.Asset{asset}, d.list.clock.Now())
	if err != nil {
		return AssetView{}, err
	}
	others, err := d.others(ctx, asset)
	if err != nil {
		return AssetView{}, err
	}
	return AssetView{Summary: sums[0], Others: others}, nil
}

func (d *Detail) others(ctx context.Context, asset domain.Asset) ([]Listing, error) {
	rows, err := d.read.OtherListings(ctx, sqlc.OtherListingsParams{
		CompanyKey: asset.CompanyKey, Symbol: asset.Symbol,
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "market.OtherListings", slog.String("symbol", asset.Symbol))
	}
	out := make([]Listing, len(rows))
	for i, row := range rows {
		item, itemErr := listingOf(row)
		if itemErr != nil {
			return nil, itemErr
		}
		out[i] = item
	}
	return out, nil
}

func listingOf(row sqlc.OtherListingsRow) (Listing, error) {
	issuer, issuerErr := domain.ParseIssuer(row.Issuer)
	kind, kindErr := domain.ParseKind(row.Kind)
	if err := errors.Join(issuerErr, kindErr); err != nil {
		return Listing{}, errs.Wrap(
			err,
			errs.CodeDecodeFailed,
			"market.OtherListings",
			slog.String("symbol", row.Symbol),
		)
	}
	return Listing{
		Symbol: row.Symbol, DisplayName: row.DisplayName, Issuer: issuer, Kind: kind,
		LogoURL: row.LogoUrl, Tradable: row.Tradable,
	}, nil
}
