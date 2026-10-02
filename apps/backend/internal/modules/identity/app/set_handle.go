package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type SetHandleDeps struct {
	UoW        *db.UnitOfWork
	Reads      sqlc.DBTX
	Hints      Hints
	Users      HandleUsers
	ClaimFacts ClaimFactsLoader
}

type HandleUsers interface {
	SetHandle(context.Context, sqlc.DBTX, ids.UserID, string, time.Time) error
}

type ClaimFactsLoader func(context.Context, sqlc.DBTX, ids.UserID, string) (sqlc.HandleClaimFactsRow, error)

type SetHandle struct{ d SetHandleDeps }

func NewSetHandle(d SetHandleDeps) *SetHandle { return &SetHandle{d: d} }

func (h *SetHandle) Handle(ctx context.Context, id ids.UserID, raw string, now time.Time) (Me, error) {
	name, err := domain.ParseHandle(raw)
	if err != nil {
		return Me{}, err
	}
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		row, err := h.d.ClaimFacts(ctx, tx.Queries(), id, name.String())
		if err != nil {
			return err
		}
		if row.Handle.Valid && row.Handle.String == name.String() {
			return nil
		}
		if err := (domain.HandlePolicy{}).Check(name, claimFacts(row, now)); err != nil {
			return err
		}
		if row.HandleTaken {
			return errs.New(errs.CodeHandleTaken, "identity.SetHandle")
		}
		if err := h.d.Users.SetHandle(ctx, tx.Queries(), id, name.String(), now); err != nil {
			return err
		}
		if err := tx.Events.Append(ctx, events.UserProfileUpdated{
			V: 1, UserID: id.UUID(), Fields: []string{"handle"}, Handle: name.String(),
		}); err != nil {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) {
			h.d.Hints.PublishHint(ctx, "user."+id.String()+".me_changed", nil)
			observability.Info(ctx, observability.IdentityHandleSet, slog.String("user_id", id.String()))
		})
		return nil
	})
	if err != nil {
		return Me{}, err
	}
	return GetMe(ctx, h.d.Reads, id)
}
