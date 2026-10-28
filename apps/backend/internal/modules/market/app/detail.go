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
	Stats   *Stats
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

type statsReader interface {
	MarketStats(context.Context, sqlc.MarketStatsParams) (sqlc.MarketStatsRow, error)
}

var _ statsReader = (*sqlc.Queries)(nil)

type Detail struct {
	list    *ListAssets
	read    detailReader
	facts   statsReader
	holding CabalsHolding
	years   *YearRanges
}

func NewDetail(db sqlc.DBTX, list *ListAssets) *Detail {
	queries := sqlc.New(db)
	return &Detail{list: list, read: queries, facts: queries, years: NewYearRanges(db)}
}

func (d *Detail) WithHistory(h PriceHistory) *Detail {
	d.years.WithHistory(h)
	return d
}

func (d *Detail) WithHolding(holding CabalsHolding) *Detail {
	d.holding = holding
	return d
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
	if profile := sums[0].Profile; profile != nil {
		rounds, roundsErr := loadRounds(ctx, d.list.profiles, profile.CompanyKey)
		if roundsErr != nil {
			return AssetView{}, roundsErr
		}
		profile.Rounds = rounds
	}
	others, err := d.others(ctx, asset)
	if err != nil {
		return AssetView{}, err
	}
	view := AssetView{Summary: sums[0], Others: others}
	if asset.Kind == domain.KindEquity {
		stats, statsErr := d.stats(ctx, sums[0], d.list.clock.Now())
		if statsErr != nil {
			return AssetView{}, statsErr
		}
		view.Stats = &stats
	}
	return view, nil
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
