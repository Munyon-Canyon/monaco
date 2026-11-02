package adapters

import (
	"context"
	"errors"
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
	cmd := app.RetryTrade{SwapID: ids.SwapIDFrom(req.Id), ActorID: user}
	if req.Body != nil {
		cmd.AtCurrentPrice = req.Body.AtCurrentPrice != nil && *req.Body.AtCurrentPrice
	}
	if err := h.Retry.Handle(ctx, cmd); err != nil {
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
	wire, err := wireSwap(got)
	if err != nil {
		return nil, err
	}
	return api.GetSwap200JSONResponse(wire), nil
}

func wireSwap(d app.SwapDetail) (api.SwapDetail, error) {
	s := d.Swap
	usdc, inErr := optionalAmount(s.InAmount)
	token, outErr := optionalAmount(s.OutAmount)
	if err := errors.Join(inErr, outErr); err != nil {
		return api.SwapDetail{}, errs.Wrap(err, errs.CodeDecodeFailed, "trading.wireSwap")
	}
	if s.Action == string(domain.ActionSell) {
		usdc, token = token, usdc
	}
	out := api.SwapDetail{
		Id: s.ID, CabalId: s.CabalID,
		Source: api.SwapSource{Kind: api.SwapDetailSourceKind(s.SourceKind), Id: s.SourceID},
		Action: api.SwapDetailAction(s.Action), Symbol: s.Symbol, AssetName: d.Asset.DisplayName,
		TokenDecimals: int(d.Asset.Decimals), UsdcMicros: usdc.int, TokenAmount: token.int, TokenAmountStr: token.text,
		Status: api.SwapDetailStatus(s.Status), TxSignature: textOrNil(s.TxSignature), CreatedAt: s.CreatedAt,
		Retryable: s.Retryable,
	}
	if s.FailureCode.Valid {
		out.FailureCode = &s.FailureCode.String
		out.FailureMessage = ptr(errs.SwapFailureMessage(s.FailureCode.String))
	}
	if s.ConfirmedAt.Valid {
		out.ConfirmedAt = &s.ConfirmedAt.Time
	}
	return out, nil
}

type optional struct {
	int  *int64
	text *string
}

func optionalAmount(raw string) (optional, error) {
	v, present, err := domain.ParseOptionalTokenAmount(raw)
	var out optional
	if err != nil || !present {
		return out, err
	}
	text := v.String()
	out.text = &text
	if n, intErr := v.Int64(); intErr == nil {
		out.int = &n
	}
	return out, nil
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
