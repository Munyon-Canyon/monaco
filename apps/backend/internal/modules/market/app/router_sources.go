package app

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type StockPrices struct {
	prices SampledPrices
	clock  clock.Clock
}

func NewStockPrices(prices SampledPrices, clk clock.Clock) StockPrices {
	return StockPrices{prices: prices, clock: clk}
}

var _ SharePrices = StockPrices{}

func (p StockPrices) SharePrice(ctx context.Context, stock multichain.StockID) (money.Micros, error) {
	id, err := domain.ParseAssetID(stock.String())
	if err != nil {
		return money.Micros{}, err
	}
	samples, err := p.prices.PricesAsOf(ctx, []domain.AssetID{id}, p.clock.Now())
	if err != nil {
		return money.Micros{}, err
	}
	sample, ok := samples[id]
	if !ok || sample.Micros.IsZero() {
		return money.Micros{}, errs.New(errs.CodeUpstreamUnavailable, "market.StockPrices.SharePrice",
			slog.String("stock_id", stock.String()))
	}
	return sample.Micros, nil
}
