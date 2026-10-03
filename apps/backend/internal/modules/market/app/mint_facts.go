package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
)

type MintFacts interface {
	Facts(ctx context.Context, mints []domain.Mint) (map[domain.Mint]MintFact, map[domain.Mint]error, error)
}

type MintFact struct {
	Decimals                     uint8
	MultiplierNum, MultiplierDen uint64

	NextMultiplierNum, NextMultiplierDen uint64
	NextMultiplierAt                     time.Time
}
