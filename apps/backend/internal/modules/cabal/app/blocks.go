package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Blocks = interface {
	BlockedEitherWay(ctx context.Context, a, b ids.UserID) (bool, error)
}

type NoBlocks struct{}

func (NoBlocks) BlockedEitherWay(context.Context, ids.UserID, ids.UserID) (bool, error) {
	return false, nil
}
