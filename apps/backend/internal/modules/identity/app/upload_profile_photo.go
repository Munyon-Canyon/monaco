package app

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type PhotoStore interface {
	Put(context.Context, string, string, []byte) (string, error)
	DeleteAll(context.Context, ids.UserID) error
}

type UploadProfilePhotoHandler struct {
	UoW   *db.UnitOfWork
	Reads sqlc.DBTX
	Clock clock.Clock
	IDs   ids.Generator
	Hints Hints
	Store PhotoStore
}

type UploadProfilePhoto = UploadProfilePhotoHandler

func (h UploadProfilePhotoHandler) Handle(
	ctx context.Context, id ids.UserID, contentType, ext string, body []byte,
) (Me, error) {
	const op = "identity.UploadProfilePhoto"
	if h.Store == nil || ext == "" || len(body) == 0 {
		return Me{}, errs.New(errs.CodePhotoInvalid, op)
	}
	url, err := h.Store.Put(ctx, id.String()+"/"+h.IDs.NewV7().String()+"."+ext, contentType, body)
	if err != nil {
		return Me{}, errs.Wrap(err, errs.CodeStorageUnavailable, op, slog.String("reason", "put"))
	}
	err = h.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		_, err := sqlc.New(tx.Queries()).SetPhotoURL(ctx, sqlc.SetPhotoURLParams{
			ID: id.UUID(), PhotoUrl: url, Now: h.Clock.Now(),
		})
		if err != nil {
			return err
		}
		if err := tx.Events.Append(ctx, events.UserProfileUpdated{
			V: 1, UserID: id.UUID(), Fields: []string{"photo"}, PhotoURL: url,
		}); err != nil {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) { h.Hints.PublishHint(ctx, "user."+id.String()+".me_changed", nil) })
		return nil
	})
	if err != nil {
		return Me{}, err
	}
	observability.Info(ctx, observability.IdentityProfileUpdated, slog.String("user_id", id.String()))
	return GetMe(ctx, h.Reads, id)
}
