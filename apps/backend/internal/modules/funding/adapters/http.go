package adapters

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/fundingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

var _ api.StrictServerInterface = HTTP{}

type HTTP struct {
	Balances port.Balances
	Wallets  app.MemberWallets
	Create   *app.CreateOnrampSessionHandler
	Exchange *app.ExchangeOnrampTokenHandler
	Report   *app.ReportOnrampStatusHandler
	Reads    sqlc.DBTX
	IDs      ids.Generator
}

func (h HTTP) GetMyBalance(
	ctx context.Context,
	_ api.GetMyBalanceRequestObject,
) (api.GetMyBalanceResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	balance, err := h.Balances.Available(ctx, user)
	if err != nil {
		return nil, err
	}
	inFlight, err := balance.InFlightFundMicros.Add(balance.InFlightWithdrawalMicros)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "funding.GetMyBalance")
	}
	address, err := h.Wallets.MemberWalletAddress(ctx, user)
	if err != nil {
		return nil, err
	}
	return api.GetMyBalance200JSONResponse(api.Balance{
		AvailableMicros: balance.AvailableMicros.String(), OnChainMicros: balance.OnChainMicros.String(),
		InFlightMicros: inFlight.String(), DepositAddress: string(address), AsOf: balance.AsOf,
	}), nil
}

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
	out, err := h.Exchange.Handle(ctx, app.ExchangeOnrampToken{Token: token})
	if err != nil {
		return nil, err
	}
	return api.ExchangeOnrampToken200JSONResponse{
		SessionId: out.SessionID, WalletAddress: string(out.WalletAddress),
		SuggestedAmountMicros: microsWire(out.SuggestedAmount), UsdcMint: out.USDCMint,
	}, nil
}

func (h HTTP) ReportOnrampStatus(
	ctx context.Context, req api.ReportOnrampStatusRequestObject,
) (api.ReportOnrampStatusResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	to, err := domain.ParseReportedOnrampStatus(string(req.Body.Status))
	if err != nil {
		return nil, err
	}
	var provider string
	if req.Body.Provider != nil {
		provider = *req.Body.Provider
	}
	session, err := h.Report.Handle(ctx, app.ReportOnrampStatus{
		SessionID: req.Id, UserID: user, To: to, Provider: provider,
	})
	if err != nil {
		return nil, err
	}
	return api.ReportOnrampStatus200JSONResponse(sessionWire(session)), nil
}

func (h HTTP) GetOnrampSession(
	ctx context.Context, req api.GetOnrampSessionRequestObject,
) (api.GetOnrampSessionResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	session, err := app.GetOnrampSession(ctx, h.Reads, req.Id, user)
	if err != nil {
		return nil, err
	}
	return api.GetOnrampSession200JSONResponse(sessionWire(session)), nil
}

func sessionWire(s app.OnrampSession) api.OnrampSession {
	return api.OnrampSession{
		SessionId:             s.ID,
		Status:                api.OnrampSessionStatus(s.Status),
		SuggestedAmountMicros: microsWire(s.SuggestedAmount),
		CreatedAt:             s.CreatedAt,
		CompletedAt:           s.CompletedAt,
	}
}

func microsWire(m *money.Micros) *string {
	if m == nil {
		return nil
	}
	s := m.String()
	return &s
}

func caller(ctx context.Context) (ids.UserID, error) {
	actor, ok := auth.ActorFrom(ctx)
	if !ok {
		return ids.UserID{}, errs.New(errs.CodeUnauthorized, "funding.caller")
	}
	if actor.Kind != auth.ActorUser {
		return ids.UserID{}, errs.New(
			errs.CodeForbidden,
			"funding.caller",
			slog.String("actor_kind", string(actor.Kind)),
		)
	}
	user, err := ids.ParseUserID(actor.ID)
	if err != nil {
		return ids.UserID{}, errs.Wrap(err, errs.CodeUnauthorized, "funding.caller")
	}
	return user, nil
}
