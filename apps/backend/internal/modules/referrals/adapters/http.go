package adapters

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/referralsapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type HTTP struct {
	Codes  app.Resolver
	Attach *app.AttachReferralHandler
}

var _ api.StrictServerInterface = HTTP{}

func (h HTTP) GetMyReferralCode(
	ctx context.Context, _ api.GetMyReferralCodeRequestObject,
) (api.GetMyReferralCodeResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	mine, err := h.Codes.MyCode(ctx, user)
	if err != nil {
		return nil, err
	}
	return api.GetMyReferralCode200JSONResponse{
		Code: mine.Code, Link: mine.Link, HandleLink: mine.HandleLink, HandleUnlocked: mine.HandleUnlocked,
	}, nil
}

func (h HTTP) PostMeReferral(
	ctx context.Context, req api.PostMeReferralRequestObject,
) (api.PostMeReferralResponseObject, error) {
	const op = "referrals.PostMeReferral"
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil || !req.Body.Source.Valid() || req.Body.Code == "" {
		return nil, errs.New(errs.CodeInvalidInput, op)
	}
	cmd := app.AttachReferral{CallerID: user, Code: req.Body.Code, Source: string(req.Body.Source)}
	if err := h.Attach.Handle(ctx, cmd); err != nil {
		return nil, err
	}
	resolved, err := h.Codes.Resolve(ctx, req.Body.Code)
	if err != nil {
		return nil, err
	}
	card, err := h.Codes.Referrer(ctx, resolved)
	if err != nil {
		return nil, err
	}
	return api.PostMeReferral201JSONResponse{Referrer: api.ReferralReferrer{
		UserId: card.ID.UUID(), DisplayName: card.DisplayName, Handle: card.Handle,
	}}, nil
}

func caller(ctx context.Context) (ids.UserID, error) {
	const op = "referrals.caller"
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
