package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/identityapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (h HTTP) BanUser(ctx context.Context, req api.BanUserRequestObject) (api.BanUserResponseObject, error) {
	reason, err := events.NewReason(req.Body.Reason)
	if err != nil {
		return nil, err
	}
	if err := h.Status.Ban(ctx, app.BanUser{UserID: ids.UserIDFrom(req.Id), Reason: reason}); err != nil {
		return nil, err
	}
	return api.BanUser204Response{}, nil
}

func (h HTTP) UnbanUser(ctx context.Context, req api.UnbanUserRequestObject) (api.UnbanUserResponseObject, error) {
	reason, err := events.NewReason(req.Body.Reason)
	if err != nil {
		return nil, err
	}
	if err := h.Status.Unban(ctx, app.UnbanUser{UserID: ids.UserIDFrom(req.Id), Reason: reason}); err != nil {
		return nil, err
	}
	return api.UnbanUser204Response{}, nil
}

func (h HTTP) RevokeHandle(
	ctx context.Context, req api.RevokeHandleRequestObject,
) (api.RevokeHandleResponseObject, error) {
	reason, err := events.NewReason(req.Body.Reason)
	if err != nil {
		return nil, err
	}
	if err := h.Handles.Revoke(ctx, app.RevokeHandle{UserID: ids.UserIDFrom(req.Id), Reason: reason}); err != nil {
		return nil, err
	}
	return api.RevokeHandle204Response{}, nil
}

func (h HTTP) ReassignHandle(
	ctx context.Context, req api.ReassignHandleRequestObject,
) (api.ReassignHandleResponseObject, error) {
	reason, err := events.NewReason(req.Body.Reason)
	if err != nil {
		return nil, err
	}
	err = h.Handles.Reassign(ctx, app.ReassignHandle{
		Handle: req.Handle, ToUserID: ids.UserIDFrom(req.Body.ToUserId), Reason: reason,
	})
	if err != nil {
		return nil, err
	}
	return api.ReassignHandle204Response{}, nil
}
