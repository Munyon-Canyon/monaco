package port

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type HolderCount struct {
	Mint   chain.SolanaAddress
	Cabals int
}

type CabalHolderCounts interface {
	CabalHolderCounts(ctx context.Context) ([]HolderCount, error)
}

type CabalsHolding interface {
	CabalsHolding(ctx context.Context, asset string) (int, error)
}
