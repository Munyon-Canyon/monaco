package adapters

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type HTTP struct {
	Create   *app.CreateOnrampSessionHandler
	Exchange *app.ExchangeOnrampTokenHandler
	IDs      ids.Generator
}

var _ httpx.FundingRoutes = HTTP{}

func (h HTTP) CreateOnrampSession(
	ctx context.Context, req api.CreateOnrampSessionRequestObject,
) (api.CreateOnrampSessionResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	var suggested *money.Micros
	if raw := req.Body.SuggestedAmountMicros; raw != nil {
		m, err := money.ParseMicros(*raw)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInvalidInput, "funding.CreateOnrampSession",
				slog.String("field", "suggested_amount_micros"))
		}
		suggested = &m
	}
	created, err := h.Create.Handle(ctx, app.CreateOnrampSession{
		ID: h.IDs.NewV7(), UserID: user, SuggestedAmount: suggested, CabalID: req.Body.CabalId,
	})
	if err != nil {
		return nil, err
	}
	return api.CreateOnrampSession201JSONResponse{
		SessionId: created.ID, Url: created.URL, ExpiresAt: created.ExpiresAt,
	}, nil
}

func (h HTTP) ExchangeOnrampToken(
	ctx context.Context, req api.ExchangeOnrampTokenRequestObject,
) (api.ExchangeOnrampTokenResponseObject, error) {
	token, err := domain.ParseOnrampToken(req.Body.Token)
	if err != nil {
		return nil, err
	}
	out, err := h.Exchange.Handle(ctx, token)
	if err != nil {
		return nil, err
	}
	return api.ExchangeOnrampToken200JSONResponse{
		SessionId: out.SessionID, WalletAddress: string(out.WalletAddress),
		SuggestedAmountMicros: microsWire(out.SuggestedAmount), UsdcMint: out.USDCMint,
	}, nil
}

func microsWire(m *money.Micros) *string {
	if m == nil {
		return nil
	}
	s := m.String()
	return &s
}

func caller(ctx context.Context) (ids.UserID, error) {
	const op = "funding.caller"
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
