package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const removePictureOp = "cabal.RemoveCabalPicture"

type RemoveCabalPicture struct {
	CabalID ids.CabalID
	Reason  events.Reason
	Admin   ids.UserID
}

type RemoveCabalPictureHandler struct {
	uow   *db.UnitOfWork
	ids   ids.Generator
	clock clock.Clock
}

func NewRemoveCabalPictureHandler(uow *db.UnitOfWork, g ids.Generator, c clock.Clock) *RemoveCabalPictureHandler {
	return &RemoveCabalPictureHandler{uow: uow, ids: g, clock: c}
}

type pictureState struct {
	HasPicture bool `json:"has_picture"`
}

func (h *RemoveCabalPictureHandler) Handle(ctx context.Context, cmd RemoveCabalPicture) error {
	return h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		_, err := q.LockCabalExclusive(ctx, cmd.CabalID.UUID())
		if errors.Is(err, sql.ErrNoRows) {
			return errs.New(errs.CodeCabalNotFound, removePictureOp)
		}
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, removePictureOp)
		}
		current, _, err := loadCabal(ctx, q, cmd.CabalID)
		if err != nil || current.PictureUrl.String == "" {
			return err
		}
		if _, err := q.SetCabalPicture(ctx, sqlc.SetCabalPictureParams{
			PictureUrl: "", Now: h.clock.Now(), ID: current.ID,
		}); err != nil {
			return errs.Wrap(err, errs.CodeInternal, removePictureOp)
		}
		cleared := ""
		if err := tx.Events.Append(ctx, events.CabalUpdated{
			V: 1, CabalID: current.ID, ActorID: cmd.Admin.UUID(), Changes: events.CabalChanges{PictureURL: &cleared},
		}); err != nil {
			return err
		}
		audit, err := events.NewAdminAction(
			h.ids.NewV7(), cmd.Admin, events.AdminActionPictureRemove, events.AdminTargetCabal,
			current.ID.String(), cmd.Reason, pictureState{HasPicture: true}, pictureState{HasPicture: false},
		)
		if err != nil {
			return err
		}
		return tx.Events.Append(ctx, audit)
	})
}
