package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type UpdateProfileHandler struct {
	UoW   *db.UnitOfWork
	Reads sqlc.DBTX
	Clock clock.Clock
	Hints Hints
}

type UpdateProfile = UpdateProfileHandler

func (h UpdateProfileHandler) Handle(ctx context.Context, id ids.UserID, raw string) (Me, error) {
	name, err := domain.ParseDisplayName(raw)
	if err != nil {
		return Me{}, err
	}
	err = h.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		n, err := sqlc.New(tx.Queries()).SetDisplayName(ctx, sqlc.SetDisplayNameParams{
			ID: id.UUID(), DisplayName: name.String(), Now: h.Clock.Now(),
		})
		if err != nil || n == 0 {
			return err
		}
		if err := tx.Events.Append(ctx, events.UserProfileUpdated{
			V: 1, UserID: id.UUID(), Fields: []string{"display_name"}, DisplayName: name.String(),
		}); err != nil {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) {
			h.Hints.PublishHint(ctx, "user."+id.String()+".me_changed", nil)
		})
		return nil
	})
	if err != nil {
		return Me{}, err
	}
	return GetMe(ctx, h.Reads, id)
}
