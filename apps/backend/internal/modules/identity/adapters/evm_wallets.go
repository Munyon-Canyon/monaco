package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

var _ app.EVMWalletStore = EVMWallets{}

type EVMWallets struct{}

func (EVMWallets) LockUser(ctx context.Context, q sqlc.DBTX, user ids.UserID) error {
	_, err := sqlc.New(q).LockUserStatus(ctx, user.UUID())
	return userErr("identity.EVMWallets.LockUser", err)
}

func (EVMWallets) StoredEVMWallet(ctx context.Context, q sqlc.DBTX, user ids.UserID) (chain.EVMAddress, bool, error) {
	const op = "identity.EVMWallets.StoredEVMWallet"
	row, err := sqlc.New(q).FindUserEVMWallet(ctx, user.UUID())
	if errors.Is(err, pgx.ErrNoRows) {
		return chain.EVMAddress{}, false, nil
	}
	if err != nil {
		return chain.EVMAddress{}, false, errs.Wrap(err, errs.CodeInternal, op)
	}
	addr, err := chain.ParseEVMAddress(row.EvmAddress)
	if err != nil {
		return chain.EVMAddress{}, false, errs.Wrap(err, errs.CodeInternal, op)
	}
	return addr, true, nil
}

func (EVMWallets) MemberAddress(ctx context.Context, q sqlc.DBTX, user ids.UserID) (chain.SolanaAddress, error) {
	row, err := sqlc.New(q).MemberWalletByUserID(ctx, user.UUID())
	if err != nil {
		return "", userErr("identity.EVMWallets.MemberAddress", err)
	}
	return chain.SolanaAddress(row.Address), nil
}

func (EVMWallets) SetEVMWallet(
	ctx context.Context, q sqlc.DBTX, user ids.UserID, wallet multichain.EVMWallet, now time.Time,
) error {
	_, err := sqlc.New(q).SetUserEVMWallet(ctx, sqlc.SetUserEVMWalletParams{
		EvmPrivyWalletID: pgtype.Text{String: wallet.ID, Valid: true},
		EvmAddress:       pgtype.Text{String: wallet.Address.String(), Valid: true},
		EvmCreatedAt:     pgtype.Timestamptz{Time: now, Valid: true},
		UserID:           user.UUID(),
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "identity.EVMWallets.SetEVMWallet")
	}
	return nil
}

func userErr(op string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return errs.New(errs.CodeUserNotFound, op)
	}
	return errs.Wrap(err, errs.CodeInternal, op)
}
