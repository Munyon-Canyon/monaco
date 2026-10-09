package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type PriceSource interface {
	Prices(ctx context.Context, mints []domain.Mint) (map[domain.Mint]money.Micros, error)
}

type CappedSource interface {
	MintsWithin(window time.Duration) int
}
