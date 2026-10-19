package app

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type UpdateProfileHandler struct {
	UoW   *db.UnitOfWork
	Reads sqlc.DBTX
	Clock clock.Clock
	Hints Hints
}

type UpdateProfile = UpdateProfileHandler

type ProfileChanges struct {
	DisplayName      *string
	Bio              *string
	SharePerformance *bool
}

func (h UpdateProfileHandler) Handle(ctx context.Context, id ids.UserID, in ProfileChanges) (Me, error) {
	if in.DisplayName == nil && in.Bio == nil && in.SharePerformance == nil {
		return Me{}, errs.New(errs.CodeInvalidInput, "identity.UpdateProfile")
	}
	var name domain.DisplayName
	if in.DisplayName != nil {
		parsed, err := domain.ParseDisplayName(*in.DisplayName)
		if err != nil {
			return Me{}, err
		}
		name = parsed
	}
	var bio domain.Bio
	if in.Bio != nil {
		parsed, err := domain.ParseBio(*in.Bio)
		if err != nil {
			return Me{}, err
		}
		bio = parsed
	}
	err := h.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		event, err := h.write(ctx, tx, id, profileWrite{name: name, bio: bio, in: in})
		if err != nil || len(event.Fields) == 0 {
			return err
		}
		if err := tx.Events.Append(ctx, event); err != nil {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) {
			h.Hints.PublishHint(ctx, "user."+id.String()+".me_changed", nil)
			observability.Info(ctx, observability.IdentityProfileUpdated, slog.String("user_id", id.String()))
		})
		return nil
	})
	if err != nil {
		return Me{}, err
	}
	return GetMe(ctx, h.Reads, id)
}

type profileWrite struct {
	name domain.DisplayName
	bio  domain.Bio
	in   ProfileChanges
}

func (h UpdateProfileHandler) write(
	ctx context.Context, tx db.Tx, id ids.UserID, w profileWrite,
) (events.UserProfileUpdated, error) {
	name, in := w.name, w.in
	q := sqlc.New(tx.Queries())
	event := events.UserProfileUpdated{V: 1, UserID: id.UUID(), Fields: []string{}}
	if in.DisplayName != nil {
		n, err := q.SetDisplayName(ctx, sqlc.SetDisplayNameParams{
			ID: id.UUID(), DisplayName: name.String(), Now: h.Clock.Now(),
		})
		if err != nil {
			return event, err
		}
		if n > 0 {
			event.Fields, event.DisplayName = append(event.Fields, "display_name"), name.String()
		}
	}
	if in.Bio != nil {
		n, err := q.SetBio(ctx, sqlc.SetBioParams{ID: id.UUID(), Bio: w.bio.String(), Now: h.Clock.Now()})
		if err != nil {
			return event, err
		}
		if n > 0 {
			event.Fields = append(event.Fields, "bio")
		}
	}
	if in.SharePerformance != nil {
		n, err := q.SetSharePerformance(ctx, sqlc.SetSharePerformanceParams{
			ID: id.UUID(), SharePerformance: *in.SharePerformance, Now: h.Clock.Now(),
		})
		if err != nil {
			return event, err
		}
		if n > 0 {
			event.Fields = append(event.Fields, "share_performance")
		}
	}
	return event, nil
}
