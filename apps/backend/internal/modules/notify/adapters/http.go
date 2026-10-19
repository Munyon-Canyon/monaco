package adapters

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/notifyapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type HTTP struct {
	Register   *app.RegisterDeviceHandler
	Unregister *app.UnregisterDeviceHandler
	SetPrefs   *app.SetNotificationPreferencesHandler
	Reads      sqlc.DBTX
	Users      app.Users
}

var _ api.StrictServerInterface = HTTP{}

func (h HTTP) PostDevice(ctx context.Context, req api.PostDeviceRequestObject) (api.PostDeviceResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	token, err := domain.ParseDeviceToken(req.Body.Token)
	if err != nil {
		return nil, err
	}
	env, err := domain.ParseEnvironment(string(req.Body.Environment))
	if err != nil {
		return nil, err
	}
	if err := h.Register.Handle(ctx, app.RegisterDevice{UserID: user, Token: token, Environment: env}); err != nil {
		return nil, err
	}
	return api.PostDevice204Response{}, nil
}

func (h HTTP) DeleteDevice(
	ctx context.Context, req api.DeleteDeviceRequestObject,
) (api.DeleteDeviceResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	token, err := domain.ParseDeviceToken(req.Token)
	if err != nil {
		return nil, err
	}
	if err := h.Unregister.Handle(ctx, app.UnregisterDevice{UserID: user, Token: token}); err != nil {
		return nil, err
	}
	return api.DeleteDevice204Response{}, nil
}

func (h HTTP) GetNotificationPreferences(
	ctx context.Context, _ api.GetNotificationPreferencesRequestObject,
) (api.GetNotificationPreferencesResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	prefs, err := app.GetNotificationPreferences(ctx, h.Reads, h.Users, user)
	if err != nil {
		return nil, err
	}
	return api.GetNotificationPreferences200JSONResponse(wire(prefs)), nil
}

func (h HTTP) PutNotificationPreferences(
	ctx context.Context, req api.PutNotificationPreferencesRequestObject,
) (api.PutNotificationPreferencesResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	prefs, err := h.SetPrefs.Handle(ctx, app.SetNotificationPreferences{
		UserID: user,
		Preferences: domain.Preferences{
			DecisionRequests:   req.Body.DecisionRequests,
			RepliesAndMentions: req.Body.RepliesAndMentions,
		},
	})
	if err != nil {
		return nil, err
	}
	return api.PutNotificationPreferences200JSONResponse(wire(prefs)), nil
}

func wire(p domain.Preferences) api.NotificationPreferences {
	return api.NotificationPreferences{DecisionRequests: p.DecisionRequests, RepliesAndMentions: p.RepliesAndMentions}
}

func caller(ctx context.Context) (ids.UserID, error) {
	const op = "notify.caller"
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
