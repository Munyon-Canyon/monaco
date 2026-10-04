package adapters

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/fundingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

var _ api.StrictServerInterface = HTTP{}

type HTTP struct {
	Balances port.Balances
	Wallets  app.MemberWallets
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
