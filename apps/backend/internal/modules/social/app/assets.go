package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type AssetCard struct {
	ID       uuid.UUID
	Symbol   string
	Name     string
	Decimals uint8
	Kind     string
}

type Treasury interface {
	PotValue(ctx context.Context, cabalID ids.CabalID) (money.Micros, error)
}

type Assets interface {
	AssetByMint(ctx context.Context, mint string) (AssetCard, error)
}

type AssetLogos interface {
	AssetLogos(ctx context.Context, assetIDs []uuid.UUID) (map[uuid.UUID]string, error)
}

type StockCard struct {
	ID          uuid.UUID
	Symbol      string
	Name        string
	LogoURL     string
	IsPreIPO    bool
	PriceMicros money.Micros
	ChangeBps   *int32
}

type StockCards interface {
	StockCards(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]StockCard, error)
}
