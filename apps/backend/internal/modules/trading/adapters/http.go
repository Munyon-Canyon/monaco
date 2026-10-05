package adapters

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/tradingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type HTTP struct {
	Retry *app.RetryTradeHandler
}

var _ api.StrictServerInterface = HTTP{}

func (h HTTP) PostSwapRetry(
	ctx context.Context, req api.PostSwapRetryRequestObject,
) (api.PostSwapRetryResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.Retry.Handle(ctx, app.RetryTrade{SwapID: ids.SwapIDFrom(req.Id), ActorID: user}); err != nil {
		return nil, err
	}
	return api.PostSwapRetry202JSONResponse{SwapId: req.Id, Status: api.RetryRequested}, nil
}

func caller(ctx context.Context) (ids.UserID, error) {
	const op = "trading.caller"
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
