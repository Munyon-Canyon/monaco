package adapters

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type HTTP struct {
	Follow   *app.FollowHandler
	Unfollow *app.UnfollowHandler
	Reads    sqlc.DBTX
}

var _ httpx.SocialRoutes = HTTP{}

func (h HTTP) PostUserFollow(
	ctx context.Context, req api.PostUserFollowRequestObject,
) (api.PostUserFollowResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	them, err := ids.ParseUserID(req.Id.String())
	if err != nil {
		return nil, err
	}
	source, err := domain.ParseClientSource(sourceOf(req.Body))
	if err != nil {
		return nil, err
	}
	if err := h.Follow.Handle(ctx, app.Follow{Follower: me, Followee: them, Source: source}); err != nil {
		return nil, err
	}
	return api.PostUserFollow200JSONResponse(api.FollowState{Following: true}), nil
}

func (h HTTP) DeleteUserFollow(
	ctx context.Context, req api.DeleteUserFollowRequestObject,
) (api.DeleteUserFollowResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	them, err := ids.ParseUserID(req.Id.String())
	if err != nil {
		return nil, err
	}
	if err := h.Unfollow.Handle(ctx, app.Unfollow{Follower: me, Followee: them}); err != nil {
		return nil, err
	}
	return api.DeleteUserFollow200JSONResponse(api.FollowState{Following: false}), nil
}

func sourceOf(body *api.FollowRequest) string {
	if body == nil || body.Source == nil {
		return ""
	}
	return *body.Source
}

func caller(ctx context.Context) (ids.UserID, error) {
	const op = "social.caller"
	actor, ok := auth.ActorFrom(ctx)
	if !ok {
		return ids.UserID{}, errs.New(errs.CodeUnauthorized, op)
	}
	if actor.Kind != auth.ActorUser {
		return ids.UserID{}, errs.New(errs.CodeForbidden, op, slog.String("actor_kind", string(actor.Kind)))
	}
	user, err := ids.ParseUserID(actor.ID)
	if err != nil {
		return ids.UserID{}, errs.Wrap(err, errs.CodeUnauthorized, op)
	}
	return user, nil
}
