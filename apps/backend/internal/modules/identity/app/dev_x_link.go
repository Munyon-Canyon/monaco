package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type DevXLinkUsers interface {
	FindByID(ctx context.Context, q sqlc.DBTX, id ids.UserID) (domain.User, error)
	UpsertDevXLink(ctx context.Context, q sqlc.DBTX, id ids.UserID, x domain.XAccount, at time.Time) error
	DeleteDevXLink(ctx context.Context, q sqlc.DBTX, id ids.UserID) error
}

type DevXLinkDeps struct {
	UoW   *db.UnitOfWork
	Reads sqlc.DBTX
	Users DevXLinkUsers
	Privy PrivyUsers
	Clock clock.Clock
}

type DevXLink struct{ d DevXLinkDeps }

func NewDevXLink(d DevXLinkDeps) *DevXLink { return &DevXLink{d: d} }

func (x *DevXLink) Link(ctx context.Context, id ids.UserID, username string) error {
	suffix, err := x.devSuffix(ctx, id)
	if err != nil {
		return err
	}
	return x.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return x.d.Users.UpsertDevXLink(ctx, tx.Queries(), id, domain.DevXAccount(suffix, username), x.d.Clock.Now())
	})
}

func (x *DevXLink) Unlink(ctx context.Context, id ids.UserID) error {
	if _, err := x.devSuffix(ctx, id); err != nil {
		return err
	}
	return x.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return x.d.Users.DeleteDevXLink(ctx, tx.Queries(), id)
	})
}

func (x *DevXLink) devSuffix(ctx context.Context, id ids.UserID) (string, error) {
	u, err := x.d.Users.FindByID(ctx, x.d.Reads, id)
	if err != nil {
		return "", err
	}
	privy, err := x.d.Privy.User(ctx, PrivyUserID(u.PrivyUserID))
	if err != nil {
		return "", err
	}
	suffix, ok := domain.DevSuffix(privy.Email)
	if !ok {
		return "", errs.New(errs.CodeNotFound, "identity.DevXLink")
	}
	return suffix, nil
}
