package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
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

func (r WalletReader) SigningWallet(ctx context.Context, user ids.UserID) (chain.Wallet, error) {
	wallet, err := r.Reader.MemberWallet(ctx, user)
	return chain.Wallet{ID: wallet.PrivyWalletID, Address: wallet.Address}, err
}

type Outflows interface {
	InFlightMicros(context.Context, ids.UserID) (money.Micros, error)
}

type WithdrawalOutflows struct{ Reads sqlc.DBTX }

func (o WithdrawalOutflows) InFlightMicros(ctx context.Context, user ids.UserID) (money.Micros, error) {
	raw, err := sqlc.New(o.Reads).InFlightWithdrawalMicros(ctx, user.UUID())
	if err != nil {
		return money.Micros{}, err
	}
	return money.ParseMicros(raw)
}
