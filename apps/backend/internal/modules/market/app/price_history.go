package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const HistoryMaxDays = -1

const VolumeMaxAge = 36 * time.Hour

type Sample struct {
	At    time.Time
	Price money.Micros
}

type Volume struct {
	At  time.Time
	USD money.Micros
}

type History struct {
	Prices    []Sample
	Volume24h *Volume
}

type PriceHistory interface {
	Configured() bool
	MarketChart(ctx context.Context, mint domain.Mint, days int) (History, error)
}

type HistoryPeeker interface {
	PriceHistory
	Peek(ctx context.Context, mint domain.Mint, days int) (History, bool, error)
}
