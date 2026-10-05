package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
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

type WithdrawalReads struct{ Reads sqlc.DBTX }

var _ port.Withdrawals = WithdrawalReads{}

func (r WithdrawalReads) OpenWithdrawals(
	ctx context.Context, user ids.UserID, page port.WithdrawalPage,
) ([]port.OpenWithdrawal, error) {
	params := sqlc.ListOpenWithdrawalsParams{UserID: user.UUID(), RowLimit: page.Limit}
	if page.Before != nil {
		params.HasCursor, params.CursorAt, params.CursorID = true, page.Before.At, page.Before.ID
	}
	rows, err := sqlc.New(r.Reads).ListOpenWithdrawals(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]port.OpenWithdrawal, len(rows))
	for i, row := range rows {
		out[i] = port.OpenWithdrawal{
			ID:        row.ID,
			Failed:    domain.WithdrawalStatus(row.Status) == domain.WithdrawalFailed,
			Delta:     money.SignedMicrosFromInt64(row.Amount),
			CreatedAt: row.CreatedAt,
		}
		if row.TxSignature.Valid {
			out[i].TxSignature = &row.TxSignature.String
		}
	}
	return out, nil
}
