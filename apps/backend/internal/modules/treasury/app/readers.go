package app

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Asset struct {
	ID                  uuid.UUID
	Symbol              string
	DisplayName         string
	Decimals            uint8
	ChainChecked        bool
	UIMultiplierNum     int64
	UIMultiplierDen     int64
	NextUIMultiplierNum int64
	NextUIMultiplierDen int64
	NextUIMultiplierAt  time.Time
}

func (a Asset) UIMultiplierAt(at time.Time) (int64, int64) {
	if a.NextUIMultiplierDen != 0 && !at.Before(a.NextUIMultiplierAt) {
		return a.NextUIMultiplierNum, a.NextUIMultiplierDen
	}
	if a.UIMultiplierDen == 0 {
		return 1, 1
	}
	return a.UIMultiplierNum, a.UIMultiplierDen
}

type Price struct {
	Micros     money.Micros
	ObservedAt time.Time
}

type PriceReader func(context.Context) (map[uuid.UUID]Price, error)

func (r PriceReader) LatestPrices(ctx context.Context) (map[uuid.UUID]Price, error) { return r(ctx) }

type MintResolver func(context.Context, chain.SolanaAddress) (Asset, error)

func (r MintResolver) AssetByMint(ctx context.Context, mint chain.SolanaAddress) (Asset, error) {
	return r(ctx, mint)
}
