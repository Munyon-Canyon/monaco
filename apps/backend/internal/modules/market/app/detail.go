package app

import (
	"context"
	"log/slog"

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
	siblingReader
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
	assets, err := siblings(ctx, d.read, asset)
	if err != nil {
		return nil, err
	}
	out := make([]Listing, len(assets))
	for i, a := range assets {
		out[i] = Listing{
			Symbol: a.Symbol, DisplayName: a.DisplayName, Issuer: a.Issuer, Kind: a.Kind,
			LogoURL: a.LogoURL, Tradable: a.Tradable(),
		}
	}
	return out, nil
}
