package port

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type CabalAsset struct {
	CabalID ids.CabalID
	Symbol  string
}

type TradeMark struct {
	SwapID           uuid.UUID
	At               time.Time
	Side             string
	Units            money.BaseUnits
	USDCMicros       money.Micros
	PriceMicros      money.Micros
	CostOfSoldMicros money.Micros
	CycleStart       time.Time
}

type TradeMarks interface {
	TradeMarks(ctx context.Context, pairs []CabalAsset) (map[CabalAsset][]TradeMark, error)
}
