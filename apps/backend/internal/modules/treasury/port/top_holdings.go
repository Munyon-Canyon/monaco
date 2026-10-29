package port

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type TopHoldingsReader interface {
	TopHoldings(ctx context.Context, cabalIDs []ids.CabalID) (map[ids.CabalID]chain.SolanaAddress, error)
}
