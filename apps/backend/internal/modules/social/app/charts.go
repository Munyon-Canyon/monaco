package app

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type ChartPoint struct {
	At          time.Time
	PriceMicros money.Micros
}

type ChartWindow struct {
	Key      uuid.UUID
	AssetID  uuid.UUID
	From, To time.Time
}

type ChartPrices interface {
	Latest(ctx context.Context, assetIDs []uuid.UUID) (map[uuid.UUID]money.Micros, error)
	Closes(ctx context.Context, windows []ChartWindow, buckets int) (map[uuid.UUID][]ChartPoint, error)
}

type ChartPair struct {
	CabalID ids.CabalID
	Symbol  string
}

type ChartTrade struct {
	At               time.Time
	Side             string
	USDCMicros       money.Micros
	PriceMicros      money.Micros
	CostOfSoldMicros money.Micros
	CycleStart       time.Time
}

type ChartTrades interface {
	TradeMarks(ctx context.Context, pairs []ChartPair) (map[ChartPair][]ChartTrade, error)
}
