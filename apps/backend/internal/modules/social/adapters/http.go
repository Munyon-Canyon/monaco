package adapters

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type HTTP struct {
	Follow        *app.FollowHandler
	Unfollow      *app.UnfollowHandler
	Mute          *app.MuteHandler
	Unmute        *app.UnmuteHandler
	PostChat      *app.PostChatMessageHandler
	DeleteChat    *app.DeleteChatMessageHandler
	Token         *app.RealtimeTokenHandler
	CreateComment *app.CreateCommentHandler
	DeleteComment *app.DeleteCommentHandler
	Members       app.Members
	Reads         sqlc.DBTX
	Users         app.Users
}

func (h HTTP) GetUserFollowers(
	ctx context.Context, req api.GetUserFollowersRequestObject,
) (api.GetUserFollowersResponseObject, error) {
	body, err := h.listFollows(ctx, req.Id, req.Params.Cursor, req.Params.Limit, true)
	if err != nil {
		return nil, err
	}
	return api.GetUserFollowers200JSONResponse(body), nil
}

func (h HTTP) GetUserFollowing(
	ctx context.Context, req api.GetUserFollowingRequestObject,
) (api.GetUserFollowingResponseObject, error) {
	body, err := h.listFollows(ctx, req.Id, req.Params.Cursor, req.Params.Limit, false)
	if err != nil {
		return nil, err
	}
	return api.GetUserFollowing200JSONResponse(body), nil
}

func (h HTTP) listFollows(
	ctx context.Context, id uuid.UUID, cursor *string, limit *int, followers bool,
) (api.FollowsPage, error) {
	me, err := caller(ctx)
	if err != nil {
		return api.FollowsPage{}, err
	}
	q := app.FollowsQuery{Viewer: me, User: ids.UserIDFrom(id), Limit: app.FollowsPageDefault}
	if limit != nil {
		q.Limit = *limit
	}
	if cursor != nil {
		after, err := domain.ParseKeyset(*cursor)
		if err != nil {
			return api.FollowsPage{}, err
		}
		q.After = &after
	}
	var page app.FollowsPage
	if followers {
		page, err = app.ListFollowers(ctx, h.Reads, h.Users, q)
	} else {
		page, err = app.ListFollowing(ctx, h.Reads, h.Users, q)
	}
	if err != nil {
		return api.FollowsPage{}, err
	}
	body := api.FollowsPage{Items: make([]api.FollowUser, len(page.Items))}
	for i, item := range page.Items {
		body.Items[i] = api.FollowUser{
			UserId:       item.User.ID.UUID(),
			Handle:       item.User.Handle,
			DisplayName:  item.User.DisplayName,
			PhotoUrl:     optionalWireText(item.User.PhotoURL),
			FollowedByMe: item.FollowedBy,
		}
	}
	if page.Next != nil {
		next := page.Next.Encode()
		body.NextCursor = &next
	}
	return body, nil
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
