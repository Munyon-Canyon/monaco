package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/port"
)

var _ port.PositionPrices = (*PositionPrices)(nil)

type PositionPrices struct {
	chart Charter
}

func NewPositionPrices(chart Charter) *PositionPrices { return &PositionPrices{chart: chart} }

func (p *PositionPrices) PositionPrices(ctx context.Context, symbol, chartRange string) ([]port.Bucketed, error) {
	chart, err := p.chart.Handle(ctx, symbol, ChartRequest{Range: chartRange})
	if err != nil {
		return nil, err
	}
	out := make([]port.Bucketed, len(chart.Points))
	for i, point := range chart.Points {
		out[i] = port.Bucketed{At: point.At, PriceMicros: point.Close}
	}
	return out, nil
}
