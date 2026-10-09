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

func (Users) DevUserByHandle(ctx context.Context, q sqlc.DBTX, handle string) (app.DevUserRow, bool, error) {
	row, err := sqlc.New(q).DevUserByHandle(ctx, pgtype.Text{String: handle, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.DevUserRow{}, false, nil
	}
	if err != nil {
		return app.DevUserRow{}, false, errs.Wrap(err, errs.CodeInternal, "identity.DevUserByHandle")
	}
	return app.DevUserRow{ID: ids.UserIDFrom(row.ID), PrivyID: row.PrivyUserID}, true, nil
}

func (Users) DevUserPrivyID(ctx context.Context, q sqlc.DBTX, id ids.UserID) (string, error) {
	privyID, err := sqlc.New(q).DevUserPrivyID(ctx, id.UUID())
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errs.New(errs.CodeNotFound, "identity.DevUserPrivyID")
	}
	if err != nil {
		return "", errs.Wrap(err, errs.CodeInternal, "identity.DevUserPrivyID")
	}
	return privyID, nil
}

func (Users) DeleteDevUser(ctx context.Context, q sqlc.DBTX, id ids.UserID) error {
	if _, err := sqlc.New(q).DeleteDevUserRows(ctx, id.UUID()); err != nil {
		return errs.Wrap(err, errs.CodeInternal, "identity.DeleteDevUser")
	}
	return nil
}
