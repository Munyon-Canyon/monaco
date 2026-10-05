package adapters

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type HTTP struct {
	Follow   *app.FollowHandler
	Unfollow *app.UnfollowHandler
	Mute     *app.MuteHandler
	Unmute   *app.UnmuteHandler
	Reads    sqlc.DBTX
}

func (h HTTP) PutMeFeedMutes(
	ctx context.Context, req api.PutMeFeedMutesRequestObject,
) (api.PutMeFeedMutesResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, errs.New(errs.CodeInvalidInput, "social.PutMeFeedMutes")
	}
	cmd := app.Mute{
		User: me, TargetType: string(req.Body.TargetType), TargetID: req.Body.TargetId,
	}
	if err := h.Mute.Handle(ctx, cmd); err != nil {
		return nil, err
	}
	return api.PutMeFeedMutes204Response{}, nil
}

func (h HTTP) DeleteMeFeedMutesTargetTypeTargetID(
	ctx context.Context, req api.DeleteMeFeedMutesTargetTypeTargetIDRequestObject,
) (api.DeleteMeFeedMutesTargetTypeTargetIDResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	cmd := app.Unmute{
		User: me, TargetType: string(req.TargetType), TargetID: req.TargetId,
	}
	if err := h.Unmute.Handle(ctx, cmd); err != nil {
		return nil, err
	}
	return api.DeleteMeFeedMutesTargetTypeTargetID204Response{}, nil
}

var _ api.StrictServerInterface = HTTP{}

func (h HTTP) GetMeFeedMutes(
	ctx context.Context, _ api.GetMeFeedMutesRequestObject,
) (api.GetMeFeedMutesResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := sqlc.New(h.Reads).ListFeedMutes(ctx, me.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "social.GetMeFeedMutes")
	}
	out := make(api.GetMeFeedMutes200JSONResponse, len(rows))
	for i, row := range rows {
		out[i] = api.FeedMute{
			TargetType: row.TargetType, TargetId: row.TargetID, CreatedAt: row.CreatedAt,
			Label: optionalWireText(row.Label.String),
		}
	}
	return out, nil
}

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
