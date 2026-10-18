package app

import (
	"context"
	"math/big"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Asset struct {
	ID                  uuid.UUID
	Symbol              string
	DisplayName         string
	Kind                string
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

type PriceReader func(context.Context, []uuid.UUID, time.Time) (map[uuid.UUID]Price, error)

func (r PriceReader) PricesAsOf(ctx context.Context, ids []uuid.UUID, at time.Time) (map[uuid.UUID]Price, error) {
	return r(ctx, ids, at)
}

type MintResolver func(context.Context, chain.SolanaAddress) (Asset, error)

func (r MintResolver) AssetByMint(ctx context.Context, mint chain.SolanaAddress) (Asset, error) {
	return r(ctx, mint)
}

type ListedAsset struct {
	Mint        string
	Symbol      string
	DisplayName string
	Kind        string
}

type ListingQuote struct {
	ListingID      uuid.UUID
	Asset          *ListedAsset
	Issuer         string
	Symbol         string
	Network        chain.Network
	Decimals       uint8
	SharesPerToken *big.Rat
	Price          *Price
	Value          func(units *big.Int) (money.Micros, error)
}

type ListingReader func(ctx context.Context, keys []string, at time.Time) (map[string]ListingQuote, error)
