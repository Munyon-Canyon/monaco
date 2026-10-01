package adapters

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type HTTP struct {
	Open  *app.OpenSessionHandler
	Reads sqlc.DBTX
}

var _ httpx.IdentityRoutes = HTTP{}

func (h HTTP) PostAuthSession(
	ctx context.Context, req api.PostAuthSessionRequestObject,
) (api.PostAuthSessionResponseObject, error) {
	header := ""
	if req.Params.Authorization != nil {
		header = *req.Params.Authorization
	}
	token, ok := httpx.BearerToken(header)
	if !ok {
		return nil, errs.New(
			errs.CodeUnauthorized,
			"identity.PostAuthSession",
			slog.String("reason", "no_bearer_token"),
		)
	}
	me, err := h.Open.Handle(ctx, app.OpenSession{Token: token})
	if err != nil {
		return nil, err
	}
	return api.PostAuthSession200JSONResponse(wireMe(me)), nil
}

func (h HTTP) GetMe(ctx context.Context, _ api.GetMeRequestObject) (api.GetMeResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	me, err := app.GetMe(ctx, h.Reads, user)
	if err != nil {
		return nil, err
	}
	return api.GetMe200JSONResponse(wireMe(me)), nil
}

func caller(ctx context.Context) (ids.UserID, error) {
	const op = "identity.caller"
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

func wireMe(m app.Me) api.Me {
	return api.Me{
		Id: m.ID.UUID(), Handle: present(m.Handle), DisplayName: m.DisplayName, PhotoUrl: present(m.PhotoURL),
		AuthState: api.AuthState(m.AuthState), AccountStatus: api.AccountStatus(m.AccountStatus),
		MemberWalletAddress: string(m.MemberWalletAddress), PhoneLinked: m.PhoneLinked,
		XUsername: present(m.XUsername), HandleChangeableAt: m.HandleChangeableAt, CreatedAt: m.CreatedAt,
	}
}

func present(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
