package app

import (
	"context"

	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type MemberWallets interface {
	MemberWalletAddress(context.Context, ids.UserID) (chain.SolanaAddress, error)
}

type WalletReader struct{ Reader identityport.WalletReader }

func (r WalletReader) MemberWalletAddress(ctx context.Context, user ids.UserID) (chain.SolanaAddress, error) {
	wallet, err := r.Reader.MemberWallet(ctx, user)
	return wallet.Address, err
}

type Outflows interface {
	InFlightMicros(context.Context, ids.UserID) (money.Micros, error)
}

type NoFundTransfers struct{}

func (NoFundTransfers) InFlightMicros(context.Context, ids.UserID) (money.Micros, error) {
	return money.Micros{}, nil
}
