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
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (Users) InsertDevUser(
	ctx context.Context, q sqlc.DBTX, id ids.UserID, privyID, handle, displayName string, at time.Time,
) error {
	return sqlc.New(q).InsertDevUser(ctx, sqlc.InsertDevUserParams{
		ID: id.UUID(), PrivyUserID: privyID, Handle: pgtype.Text{String: handle, Valid: true},
		DisplayName: displayName, Now: at,
	})
}

var _ app.DevPools = Users{}

func (Users) LockDevHandle(ctx context.Context, q sqlc.DBTX, handle string) error {
	if err := sqlc.New(q).LockDevHandle(ctx, handle); err != nil {
		return errs.Wrap(err, errs.CodeInternal, "identity.LockDevHandle")
	}
	return nil
}

func (Users) DevUserByHandle(ctx context.Context, q sqlc.DBTX, handle string) (app.DevUserRow, bool, error) {
	row, err := sqlc.New(q).DevUserByHandle(ctx, pgtype.Text{String: handle, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.DevUserRow{}, false, nil
	}
	if err != nil {
		return app.DevUserRow{}, false, errs.Wrap(err, errs.CodeInternal, "identity.DevUserByHandle")
	}
	return app.DevUserRow{ID: ids.UserIDFrom(row.ID), PrivyID: row.PrivyUserID, Deleted: row.Deleted}, true, nil
}

func (Users) RestoreDevUser(
	ctx context.Context, q sqlc.DBTX, id ids.UserID, displayName string, at time.Time,
) (bool, error) {
	n, err := sqlc.New(q).
		RestoreDevUser(ctx, sqlc.RestoreDevUserParams{DisplayName: displayName, Now: at, ID: id.UUID()})
	if err != nil {
		return false, errs.Wrap(err, errs.CodeInternal, "identity.RestoreDevUser")
	}
	return n == 1, nil
}

var _ app.DevUserEraser = Users{}

func (Users) DevUserForDelete(ctx context.Context, q sqlc.DBTX, id ids.UserID) (app.DevUserDeletion, bool, error) {
	row, err := sqlc.New(q).DevUserForDelete(ctx, id.UUID())
	if errors.Is(err, pgx.ErrNoRows) {
		return app.DevUserDeletion{}, false, nil
	}
	if err != nil {
		return app.DevUserDeletion{}, false, errs.Wrap(err, errs.CodeInternal, "identity.DevUserForDelete")
	}
	return app.DevUserDeletion{
		Handle:        row.Handle,
		PrivyID:       row.PrivyUserID,
		WalletAddress: row.WalletAddress,
	}, true, nil
}

func (Users) DeleteDevUser(ctx context.Context, q sqlc.DBTX, id ids.UserID) error {
	if _, err := sqlc.New(q).DeleteDevUserRows(ctx, id.UUID()); err != nil {
		return errs.Wrap(err, errs.CodeInternal, "identity.DeleteDevUser")
	}
	return nil
}
