package app

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func adminFrom(ctx context.Context, op string) (ids.UserID, error) {
	actor, ok := auth.ActorFrom(ctx)
	switch {
	case !ok:
		return ids.UserID{}, errs.New(errs.CodeUnauthorized, op)
	case actor.Kind != auth.ActorAdmin:
		return ids.UserID{}, errs.New(errs.CodeForbidden, op, slog.String("actor_kind", string(actor.Kind)))
	}
	admin, err := ids.ParseUserID(actor.ID)
	if err != nil {
		return ids.UserID{}, errs.Wrap(err, errs.CodeAdminForbidden, op)
	}
	return admin, nil
}
