package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (h HTTP) PostUserBlock(
	ctx context.Context, req api.PostUserBlockRequestObject,
) (api.PostUserBlockResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	them, err := ids.ParseUserID(req.Id.String())
	if err != nil {
		return nil, err
	}
	if err := h.Block.Handle(ctx, app.BlockUser{Blocker: me, Blocked: them}); err != nil {
		return nil, err
	}
	return api.PostUserBlock204Response{}, nil
}

func (h HTTP) DeleteUserBlock(
	ctx context.Context, req api.DeleteUserBlockRequestObject,
) (api.DeleteUserBlockResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	them, err := ids.ParseUserID(req.Id.String())
	if err != nil {
		return nil, err
	}
	if err := h.Unblock.Handle(ctx, app.UnblockUser{Blocker: me, Blocked: them}); err != nil {
		return nil, err
	}
	return api.DeleteUserBlock204Response{}, nil
}

func (h HTTP) GetMeBlocks(
	ctx context.Context, req api.GetMeBlocksRequestObject,
) (api.GetMeBlocksResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	q := app.BlocksQuery{Viewer: me, Limit: app.BlocksPageMax}
	if req.Params.Limit != nil {
		q.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		after, err := domain.ParseKeyset(*req.Params.Cursor)
		if err != nil {
			return nil, err
		}
		q.After = &after
	}
	page, err := app.ListBlocks(ctx, h.Reads, h.Users, q)
	if err != nil {
		return nil, err
	}
	body := api.BlockedUsers{Users: make([]api.BlockedUser, len(page.Items))}
	if page.Next != nil {
		next := page.Next.Encode()
		body.NextCursor = &next
	}
	for i, b := range page.Items {
		user := api.BlockedUser{UserId: b.ID.UUID()}
		if !b.Card.Deleted {
			user.Handle, user.DisplayName, user.PhotoUrl = b.Card.Handle, b.Card.DisplayName, optionalWireText(
				b.Card.PhotoURL,
			)
		}
		body.Users[i] = user
	}
	return api.GetMeBlocks200JSONResponse(body), nil
}
