package events

import (
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const TypePriceTick Type = "price.tick"

type PriceTick struct {
	V      int         `json:"v"`
	AsOf   time.Time   `json:"as_of"`
	Prices []TickPrice `json:"prices"`
}

type TickPrice struct {
	Mint        chain.SolanaAddress `json:"mint"`
	AssetID     uuid.UUID           `json:"asset_id"`
	PriceMicros money.Micros        `json:"price_micros"`
	ObservedAt  time.Time           `json:"observed_at"`
}

func (PriceTick) Type() Type { return TypePriceTick }

func (PriceTick) core() {}
