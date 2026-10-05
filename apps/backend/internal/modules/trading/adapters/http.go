package adapters

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/tradingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type HTTP struct {
	Retry *app.RetryTradeHandler
	Swaps app.SwapDetailReads
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

func (h HTTP) GetSwap(ctx context.Context, req api.GetSwapRequestObject) (api.GetSwapResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	got, err := h.Swaps.Swap(ctx, ids.SwapIDFrom(req.Id), user)
	if err != nil {
		return nil, err
	}
	return api.GetSwap200JSONResponse(wireSwap(got)), nil
}

func wireSwap(d app.SwapDetail) api.SwapDetail {
	s := d.Swap
	usdc, token := &s.InAmount, nullable(s.OutAmount)
	if s.Action == string(domain.ActionSell) {
		usdc, token = token, usdc
	}
	out := api.SwapDetail{
		Id: s.ID, CabalId: s.CabalID,
		Source: api.SwapSource{Kind: api.SwapDetailSourceKind(s.SourceKind), Id: s.SourceID},
		Action: api.SwapDetailAction(s.Action), Symbol: s.Symbol, AssetName: d.Asset.DisplayName,
		TokenDecimals: int(d.Asset.Decimals), UsdcMicros: usdc, TokenAmount: token,
		Status: api.SwapDetailStatus(s.Status), TxSignature: textOrNil(s.TxSignature), CreatedAt: s.CreatedAt,
		Retryable: s.Retryable,
	}
	if s.FailureCode.Valid {
		out.FailureCode = &s.FailureCode.String
		out.FailureMessage = ptr(errs.Message(errs.CodeSwapFailed))
	}
	if s.ConfirmedAt.Valid {
		out.ConfirmedAt = &s.ConfirmedAt.Time
	}
	return out
}

func nullable(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

func textOrNil(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func ptr[T any](v T) *T { return &v }

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
