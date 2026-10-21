package adapters

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type RealtimeRevoke struct{ Realtime app.Realtime }

func (h RealtimeRevoke) Left(ctx context.Context, e events.CabalMemberLeft) (struct{}, error) {
	return struct{}{}, h.revoke(ctx, e.UserID)
}

func (h RealtimeRevoke) Standing(ctx context.Context, e events.UserAccountStatusChanged) (struct{}, error) {
	if domain.AccountStatus(e.To) == domain.AccountActive {
		return struct{}{}, nil
	}
	return struct{}{}, h.revoke(ctx, e.UserID)
}

func (h RealtimeRevoke) Deleted(ctx context.Context, e events.UserDeleted) (struct{}, error) {
	return struct{}{}, h.revoke(ctx, e.UserID)
}

func (h RealtimeRevoke) revoke(ctx context.Context, user uuid.UUID) error {
	if err := h.Realtime.RevokeTokens(ctx, ids.UserIDFrom(user)); err != nil {
		return errs.Wrap(err, errs.CodeOf(err), "social.RealtimeRevoke")
	}
	return nil
}

func Settled[E events.Event](context.Context, db.Tx, E, struct{}, time.Time) error { return nil }
