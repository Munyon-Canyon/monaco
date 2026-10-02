package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type TreasuryWallets interface {
	CreateAppOwned(ctx context.Context, key string) (walletID string, address chain.SolanaAddress, err error)
}
