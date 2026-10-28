package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/treasuryapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func (h HTTP) FundCabal(ctx context.Context, req api.FundCabalRequestObject) (api.FundCabalResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, errs.New(errs.CodeInvalidInput, "treasury.FundCabal")
	}
	amount, err := domain.ParseFundAmount(req.Body.AmountMicros)
	if err != nil {
		return nil, err
	}
	id, err := h.Fund.Handle(ctx, app.FundCabal{
		CabalID: ids.CabalIDFrom(req.Id), UserID: user, Amount: amount,
		Intent: httpx.IntentKey(ctx, app.FundCabalOperation),
	})
	if err != nil {
		return nil, err
	}
	return api.FundCabal202JSONResponse{TransferId: id, Status: api.FundTransferStatus(domain.FundSubmitted)}, nil
}

func (h HTTP) GetFundPreview(
	ctx context.Context, req api.GetFundPreviewRequestObject,
) (api.GetFundPreviewResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	var amount *money.Micros
	if req.Params.AmountMicros != nil {
		parsed, err := money.ParseMicros(*req.Params.AmountMicros)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInvalidInput, "treasury.GetFundPreview")
		}
		amount = &parsed
	}
	if h.FundPreview == nil {
		return nil, errs.New(errs.CodeUpstreamUnavailable, "treasury.GetFundPreview")
	}
	preview, err := h.FundPreview.Handle(ctx, user, ids.CabalIDFrom(req.Id), amount)
	if err != nil {
		return nil, err
	}
	return api.GetFundPreview200JSONResponse{
		AvailableMicros: preview.AvailableMicros.String(), MinMicros: preview.MinMicros.String(),
		SliceBpsNow: preview.SliceBpsNow, SliceBpsAfter: preview.SliceBpsAfter,
		OverMicros: preview.OverMicros.String(), Paused: preview.Paused, FirstFundedAt: preview.FirstFundedAt,
	}, nil
}

func (h HTTP) GetFundTransfer(
	ctx context.Context, req api.GetFundTransferRequestObject,
) (api.GetFundTransferResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	view, err := GetFundTransfer(ctx, h.FundReads, h.Pot, user, req.Id)
	if err != nil {
		return nil, err
	}
	return api.GetFundTransfer200JSONResponse{
		Status: api.FundTransferStatus(view.Status), AmountMicros: view.AmountMicros,
		ShareUnits: optional(view.ShareUnits), SliceBps: view.SliceBps, FailCode: optional(view.FailCode),
	}, nil
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
