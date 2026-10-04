package adapters

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type DevX struct {
	app.PrivyUsers
	Reads sqlc.DBTX
}

func (d DevX) User(ctx context.Context, id app.PrivyUserID) (app.PrivyUser, error) {
	u, err := d.PrivyUsers.User(ctx, id)
	if err != nil {
		return u, err
	}
	if _, dev := domain.DevSuffix(u.Email); !dev {
		return u, nil
	}
	row, err := sqlc.New(d.Reads).DevXLinkByPrivyUserID(ctx, string(id))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return u, nil
	case err != nil:
		return app.PrivyUser{}, err
	}
	u.X = &domain.XAccount{UserID: row.XUserID, Username: row.XUsername}
	return u, nil
}

func (Users) UpsertDevXLink(ctx context.Context, q sqlc.DBTX, id ids.UserID, x domain.XAccount, at time.Time) error {
	return sqlc.New(q).UpsertDevXLink(ctx, sqlc.UpsertDevXLinkParams{
		UserID: id.UUID(), XUserID: x.UserID, XUsername: x.Username, Now: at,
	})
}

func (Users) DeleteDevXLink(ctx context.Context, q sqlc.DBTX, id ids.UserID) error {
	return sqlc.New(q).DeleteDevXLink(ctx, id.UUID())
}
