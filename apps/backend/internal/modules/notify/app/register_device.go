package app

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type Users interface {
	UsersByID(ctx context.Context, userIDs []ids.UserID) (map[ids.UserID]identity.UserCard, error)
}

type RegisterDevice struct {
	UserID      ids.UserID
	Token       domain.DeviceToken
	Environment domain.Environment
}

type RegisterDeviceHandler struct {
	uow   *db.UnitOfWork
	users Users
	ids   ids.Generator
	clock clock.Clock
}

func NewRegisterDeviceHandler(
	uow *db.UnitOfWork, users Users, g ids.Generator, c clock.Clock,
) *RegisterDeviceHandler {
	return &RegisterDeviceHandler{uow: uow, users: users, ids: g, clock: c}
}

func (h *RegisterDeviceHandler) Handle(ctx context.Context, cmd RegisterDevice) error {
	const op = "notify.RegisterDevice"
	cards, err := h.users.UsersByID(ctx, []ids.UserID{cmd.UserID})
	if err != nil {
		return err
	}
	if card, ok := cards[cmd.UserID]; !ok || card.Deleted {
		return errs.New(errs.CodeUnauthorized, op, slog.String("user_id", cmd.UserID.String()))
	}
	row := sqlc.RegisterTokenParams{
		ID:          h.ids.NewV7(),
		UserID:      cmd.UserID.UUID(),
		Token:       cmd.Token.String(),
		Environment: string(cmd.Environment),
		CreatedAt:   h.clock.Now(),
	}
	if err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return sqlc.New(tx.Queries()).RegisterToken(ctx, row)
	}); err != nil {
		return err
	}
	observability.Info(ctx, observability.NotifyDeviceRegistered,
		slog.String("user_id", cmd.UserID.String()), slog.String("environment", row.Environment))
	return nil
}
