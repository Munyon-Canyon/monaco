package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/treasuryapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
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
	id, err := h.Fund.Handle(ctx, app.FundCabal{CabalID: ids.CabalIDFrom(req.Id), UserID: user, Amount: amount})
	if err != nil {
		return nil, err
	}
	return api.FundCabal202JSONResponse{TransferId: id, Status: api.FundTransferStatus(domain.FundSubmitted)}, nil
}

func (h HTTP) GetFundTransfer(
	ctx context.Context, req api.GetFundTransferRequestObject,
) (api.GetFundTransferResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	view, err := GetFundTransfer(ctx, h.FundReads, user, req.Id)
	if err != nil {
		return nil, err
	}
	return api.GetFundTransfer200JSONResponse{
		Status: api.FundTransferStatus(view.Status), AmountMicros: view.AmountMicros,
		ShareUnits: optional(view.ShareUnits), FailCode: optional(view.FailCode),
	}, nil
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
