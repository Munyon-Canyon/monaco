package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type AppWalletCreator interface {
	CreateAppWallet(ctx context.Context, idempotencyKey string) (chain.Wallet, error)
}

type AppWallets struct {
	Client AppWalletCreator
}

func (a AppWallets) CreateAppOwned(
	ctx context.Context, key string,
) (string, chain.SolanaAddress, error) {
	const op = "cabal.TreasuryWallets.CreateAppOwned"
	wallet, err := a.Client.CreateAppWallet(ctx, key)
	if err != nil {
		if errs.KindOf(errs.CodeOf(err)) == errs.KindUnavailable {
			return "", "", errs.Wrap(err, errs.CodePrivyUnavailable, op)
		}
		return "", "", err
	}
	return wallet.ID, wallet.Address, nil
}
