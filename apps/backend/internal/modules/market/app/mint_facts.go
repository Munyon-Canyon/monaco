package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
)

type MintFacts interface {
	Facts(ctx context.Context, mint domain.Mint) (decimals uint8, multiplierNum, multiplierDen uint64, err error)
}
