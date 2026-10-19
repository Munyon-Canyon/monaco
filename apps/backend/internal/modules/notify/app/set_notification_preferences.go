package app

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type SetNotificationPreferences struct {
	UserID      ids.UserID
	Preferences domain.Preferences
}

type SetNotificationPreferencesHandler struct {
	uow   *db.UnitOfWork
	users Users
	clock clock.Clock
}

func NewSetNotificationPreferencesHandler(
	uow *db.UnitOfWork, users Users, c clock.Clock,
) *SetNotificationPreferencesHandler {
	return &SetNotificationPreferencesHandler{uow: uow, users: users, clock: c}
}

func (h *SetNotificationPreferencesHandler) Handle(
	ctx context.Context, cmd SetNotificationPreferences,
) (domain.Preferences, error) {
	if err := requireLiveUser(ctx, h.users, cmd.UserID, "notify.SetNotificationPreferences"); err != nil {
		return domain.Preferences{}, err
	}
	now := h.clock.Now()
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		for _, c := range domain.Categories() {
			if err := q.UpsertPreference(ctx, sqlc.UpsertPreferenceParams{
				UserID: cmd.UserID.UUID(), Category: string(c), Enabled: cmd.Preferences.Enabled(c), UpdatedAt: now,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return domain.Preferences{}, err
	}
	return cmd.Preferences, nil
}

func requireLiveUser(ctx context.Context, users Users, user ids.UserID, op string) error {
	cards, err := users.UsersByID(ctx, []ids.UserID{user})
	if err != nil {
		return err
	}
	if card, ok := cards[user]; !ok || card.Deleted {
		return errs.New(errs.CodeUnauthorized, op, slog.String("user_id", user.String()))
	}
	return nil
}
