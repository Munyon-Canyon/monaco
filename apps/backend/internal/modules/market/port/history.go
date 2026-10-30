package port

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Bucketed struct {
	At          time.Time
	PriceMicros money.Micros
}

type Window struct {
	Key      uuid.UUID
	AssetID  uuid.UUID
	From, To time.Time
}

type History interface {
	Latest(ctx context.Context, assetIDs []uuid.UUID) (map[uuid.UUID]money.Micros, error)
	Closes(ctx context.Context, windows []Window, buckets int) (map[uuid.UUID][]Bucketed, error)
}

type PositionPrices interface {
	PositionPrices(ctx context.Context, symbol, chartRange string) ([]Bucketed, error)
}
