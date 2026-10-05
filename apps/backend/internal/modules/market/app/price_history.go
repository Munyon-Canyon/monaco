package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Sample struct {
	At    time.Time
	Price money.Micros
}

type PriceHistory interface {
	Configured() bool
	MarketChart(ctx context.Context, mint domain.Mint, days int) ([]Sample, error)
}
