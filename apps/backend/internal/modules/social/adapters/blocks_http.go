package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
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
