package adapters

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Users struct {
	Clock clock.Clock
}

func (r Users) Insert(ctx context.Context, q sqlc.DBTX, u domain.NewUser) error {
	now := r.Clock.Now()
	queries := sqlc.New(q)
	if err := queries.InsertUser(ctx, sqlc.InsertUserParams{
		ID: u.ID.UUID(), PrivyUserID: u.PrivyUserID, LoginProvider: string(u.LoginProvider),
		Email: pgtype.Text{String: u.Email, Valid: u.Email != ""}, Now: now,
	}); err != nil {
		return err
	}
	if u.Wallet == nil {
		return nil
	}
	return queries.InsertUserWallet(ctx, sqlc.InsertUserWalletParams{
		UserID: u.ID.UUID(), PrivyWalletID: u.Wallet.PrivyWalletID, Address: string(u.Wallet.Address), CreatedAt: now,
	})
}

func (Users) FindByPrivyUserID(ctx context.Context, q sqlc.DBTX, privyUserID string) (domain.User, error) {
	row, err := sqlc.New(q).FindUserByPrivyUserID(ctx, privyUserID)
	if err != nil {
		return domain.User{}, notFound(err, "identity.Users.FindByPrivyUserID")
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

func (r Users) UpdateAuthState(ctx context.Context, q sqlc.DBTX, id ids.UserID, expected, next domain.AuthState) error {
	n, err := sqlc.New(q).UpdateAuthState(ctx, sqlc.UpdateAuthStateParams{
		Next: string(next), Now: r.Clock.Now(), ID: id.UUID(), Expected: string(expected),
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

func (r Users) UpdateAccountStatus(
	ctx context.Context, q sqlc.DBTX, id ids.UserID, expected, next domain.AccountStatus,
) error {
	n, err := sqlc.New(q).UpdateAccountStatus(ctx, sqlc.UpdateAccountStatusParams{
		Next: string(next), Now: r.Clock.Now(), ID: id.UUID(), Expected: string(expected),
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
