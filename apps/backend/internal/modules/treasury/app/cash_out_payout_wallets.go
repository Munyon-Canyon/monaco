package app

import (
	"context"

	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type TreasuryWallets interface {
	TreasuryWallet(ctx context.Context, id ids.CabalID) (cabalport.TreasuryWallet, error)
}

type MemberWallets interface {
	MemberWallet(ctx context.Context, id ids.UserID) (identityport.MemberWallet, error)
}

type PayoutWalletReads struct {
	Cabals  TreasuryWallets
	Members MemberWallets
}

func (w PayoutWalletReads) TreasuryWallet(ctx context.Context, cabal ids.CabalID) (chain.Wallet, error) {
	tw, err := w.Cabals.TreasuryWallet(ctx, cabal)
	if err != nil {
		return chain.Wallet{}, err
	}
	return chain.Wallet{ID: tw.PrivyWalletID, Address: tw.Address}, nil
}

func (w PayoutWalletReads) MemberAddress(ctx context.Context, user ids.UserID) (chain.SolanaAddress, error) {
	mw, err := w.Members.MemberWallet(ctx, user)
	return mw.Address, err
}
