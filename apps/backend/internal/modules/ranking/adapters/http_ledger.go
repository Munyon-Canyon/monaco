package adapters

import (
	"context"
	"encoding/json"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
)

func (h HTTP) GetMyLatestLedger(
	ctx context.Context, _ api.GetMyLatestLedgerRequestObject,
) (api.GetMyLatestLedgerResponseObject, error) {
	viewer, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	payload, err := h.Ledgers.Latest(ctx, viewer)
	if err != nil {
		return nil, err
	}
	ledger, err := decodeLedger(payload)
	if err != nil {
		return nil, err
	}
	return api.GetMyLatestLedger200JSONResponse(ledger), nil
}

func (h HTTP) GetMyLedger(
	ctx context.Context, req api.GetMyLedgerRequestObject,
) (api.GetMyLedgerResponseObject, error) {
	viewer, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	payload, err := h.Ledgers.Week(ctx, viewer, req.Week)
	if err != nil {
		return nil, err
	}
	ledger, err := decodeLedger(payload)
	if err != nil {
		return nil, err
	}
	return api.GetMyLedger200JSONResponse(ledger), nil
}

func decodeLedger(payload []byte) (api.Ledger, error) {
	var ledger api.Ledger
	if err := json.Unmarshal(payload, &ledger); err != nil {
		return api.Ledger{}, errs.Wrap(err, errs.CodeInternal, "ranking.decodeLedger")
	}
	return ledger, nil
}
