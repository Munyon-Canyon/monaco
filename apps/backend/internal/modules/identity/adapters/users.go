package adapters

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Users struct{}

func (Users) Create(ctx context.Context, q sqlc.DBTX, u domain.NewUser, at time.Time) (bool, error) {
	n, err := sqlc.New(q).CreateUser(ctx, sqlc.CreateUserParams{
		ID: u.ID.UUID(), PrivyUserID: u.PrivyUserID, LoginProvider: string(u.LoginProvider), Now: at,
	})
	return n == 1, err
}

func (Users) AttachWallet(
	ctx context.Context, q sqlc.DBTX, id ids.UserID, w domain.Wallet, at time.Time,
) (bool, error) {
	n, err := sqlc.New(q).AttachUserWallet(ctx, sqlc.AttachUserWalletParams{
		UserID: id.UUID(), PrivyWalletID: w.PrivyWalletID, Address: string(w.Address), CreatedAt: at,
	})
	return n == 1, err
}

func (Users) RefreshEmail(ctx context.Context, q sqlc.DBTX, id ids.UserID, email string, at time.Time) error {
	return sqlc.New(q).SetUserEmail(ctx, sqlc.SetUserEmailParams{
		ID: id.UUID(), Email: pgtype.Text{String: email, Valid: email != ""}, Now: at,
	})
}

func (Users) FindByPrivyUserID(ctx context.Context, q sqlc.DBTX, privyUserID string) (domain.User, error) {
	row, err := sqlc.New(q).FindUserByPrivyUserID(ctx, privyUserID)
	if err != nil {
		return domain.User{}, notFound(err, "identity.Users.FindByPrivyUserID")
	}
	return userFrom(sqlc.FindUserByIDRow(row))
}

func (Users) Lock(ctx context.Context, q sqlc.DBTX, privyUserID string) (domain.User, error) {
	row, err := sqlc.New(q).LockUserByPrivyUserID(ctx, privyUserID)
	if err != nil {
		return domain.User{}, notFound(err, "identity.Users.Lock")
	}
	return userFrom(sqlc.FindUserByIDRow(row))
}

func (Users) FindByID(ctx context.Context, q sqlc.DBTX, id ids.UserID) (domain.User, error) {
	row, err := sqlc.New(q).FindUserByID(ctx, id.UUID())
	if err != nil {
		return domain.User{}, notFound(err, "identity.Users.FindByID")
	}
	return userFrom(row)
}

func (Users) UpdateAuthState(
	ctx context.Context, q sqlc.DBTX, id ids.UserID, expected, next domain.AuthState, at time.Time,
) error {
	n, err := sqlc.New(q).UpdateAuthState(ctx, sqlc.UpdateAuthStateParams{
		Next: string(next), Now: at, ID: id.UUID(), Expected: string(expected),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return errs.New(errs.CodeAuthStateTransition, "identity.Users.UpdateAuthState",
			slog.String("expected", string(expected)), slog.String("next", string(next)))
	}
	return nil
}

func (Users) UpdateAccountStatus(
	ctx context.Context, q sqlc.DBTX, id ids.UserID, expected, next domain.AccountStatus, at time.Time,
) error {
	n, err := sqlc.New(q).UpdateAccountStatus(ctx, sqlc.UpdateAccountStatusParams{
		Next: string(next), Now: at, ID: id.UUID(), Expected: string(expected),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return errs.New(errs.CodeAccountStatusTransition, "identity.Users.UpdateAccountStatus",
			slog.String("expected", string(expected)), slog.String("next", string(next)))
	}
	return nil
}

func notFound(err error, op string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return errs.New(errs.CodeUserNotFound, op)
	}
	return err
}

func userFrom(row sqlc.FindUserByIDRow) (domain.User, error) {
	id, err := ids.ParseUserID(row.ID.String())
	if err != nil {
		return domain.User{}, errs.Wrap(err, errs.CodeDecodeFailed, "identity.userFrom")
	}
	u := domain.User{
		ID: id, PrivyUserID: row.PrivyUserID, Handle: row.Handle.String,
		AuthState: domain.AuthState(row.AuthState), AccountStatus: domain.AccountStatus(row.AccountStatus),
	}
	if row.PrivyWalletID.Valid {
		u.Wallet = &domain.Wallet{
			PrivyWalletID: row.PrivyWalletID.String, Address: chain.SolanaAddress(row.Address.String),
		}
	}
	return u, nil
}
