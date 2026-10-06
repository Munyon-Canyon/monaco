package app

import (
	"context"

	"github.com/google/uuid"
)

type AssetCard struct {
	ID       uuid.UUID
	Symbol   string
	Name     string
	Decimals uint8
}

type Assets interface {
	AssetByMint(ctx context.Context, mint string) (AssetCard, error)
}
