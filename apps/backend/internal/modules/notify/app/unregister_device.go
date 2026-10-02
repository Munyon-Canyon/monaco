package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type UnregisterDevice struct {
	UserID ids.UserID
	Token  domain.DeviceToken
}

type UnregisterDeviceHandler struct {
	uow *db.UnitOfWork
}

func NewUnregisterDeviceHandler(uow *db.UnitOfWork) *UnregisterDeviceHandler {
	return &UnregisterDeviceHandler{uow: uow}
}

func (h *UnregisterDeviceHandler) Handle(ctx context.Context, cmd UnregisterDevice) error {
	var env string
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		removed, err := sqlc.New(tx.Queries()).DeleteOwnToken(ctx,
			sqlc.DeleteOwnTokenParams{Token: cmd.Token.String(), UserID: cmd.UserID.UUID()})
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		env = removed
		return err
	})
	switch {
	case err != nil:
		return err
	case env == "":
		observability.Info(
			ctx,
			observability.NotifyDeviceUnregisterSkipped,
			slog.String("user_id", cmd.UserID.String()),
		)
	default:
		observability.Info(ctx, observability.NotifyDeviceUnregistered,
			slog.String("user_id", cmd.UserID.String()), slog.String("environment", env))
	}
	return nil
}
