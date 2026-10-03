package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const setPictureOp = "cabal.SetCabalPicture"

type PictureStore interface {
	Put(ctx context.Context, key, contentType string, body []byte) (string, error)
}

type Picture struct {
	ContentType string
	Ext         string
	Body        []byte
}

type SetCabalPicture struct {
	ActorID ids.UserID
	CabalID ids.CabalID
	Picture *Picture
}

type SetCabalPictureHandler struct {
	uow   *db.UnitOfWork
	reads sqlc.DBTX
	ids   ids.Generator
	clock clock.Clock
	store PictureStore
}

func NewSetCabalPictureHandler(
	uow *db.UnitOfWork, reads sqlc.DBTX, g ids.Generator, c clock.Clock, store PictureStore,
) *SetCabalPictureHandler {
	return &SetCabalPictureHandler{uow: uow, reads: reads, ids: g, clock: c, store: store}
}

func (h *SetCabalPictureHandler) Handle(ctx context.Context, cmd SetCabalPicture) error {
	url, err := h.upload(ctx, cmd)
	if err != nil {
		return err
	}
	return h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		current, _, err := lockForEdit(ctx, q, cmd.CabalID, cmd.ActorID)
		if err != nil || current.PictureUrl.String == url {
			return err
		}
		if _, err := q.SetCabalPicture(ctx, sqlc.SetCabalPictureParams{
			PictureUrl: url, Now: h.clock.Now(), ID: current.ID,
		}); err != nil {
			return errs.Wrap(err, errs.CodeInternal, setPictureOp)
		}
		return tx.Events.Append(ctx, events.CabalUpdated{
			V: 1, CabalID: current.ID, ActorID: cmd.ActorID.UUID(), Changes: events.CabalChanges{PictureURL: &url},
		})
	})
}

func (h *SetCabalPictureHandler) upload(ctx context.Context, cmd SetCabalPicture) (string, error) {
	if cmd.Picture == nil {
		return "", nil
	}
	if _, _, err := editable(ctx, sqlc.New(h.reads), cmd.CabalID, cmd.ActorID); err != nil {
		return "", err
	}
	if h.store == nil {
		return "", errs.New(errs.CodeStorageUnavailable, setPictureOp)
	}
	key := "cabals/" + cmd.CabalID.String() + "/" + h.ids.NewV7().String() + "." + cmd.Picture.Ext
	url, err := h.store.Put(ctx, key, cmd.Picture.ContentType, cmd.Picture.Body)
	if err != nil {
		return "", errs.Wrap(err, errs.CodeStorageUnavailable, setPictureOp)
	}
	return url, nil
}
