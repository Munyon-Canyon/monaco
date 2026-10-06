package app

import (
	"context"

	"github.com/google/uuid"
)

type AssetCard struct {
	ID   uuid.UUID
	Name string
}

type Assets interface {
	AssetByMint(ctx context.Context, mint string) (AssetCard, error)
}
