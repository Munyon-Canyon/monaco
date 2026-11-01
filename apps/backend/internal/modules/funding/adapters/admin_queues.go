package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/fundingapi"
)

const defaultBouncePage = 50

func (h HTTP) GetBounceQueue(
	ctx context.Context, req api.GetBounceQueueRequestObject,
) (api.GetBounceQueueResponseObject, error) {
	limit := defaultBouncePage
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	pending, err := app.BounceQueue(ctx, h.Reads, app.OpenBounceStatuses(), limit)
	if err != nil {
		return nil, err
	}
	body := api.BounceQueue{Items: make([]api.PendingBounce, len(pending))}
	for i, p := range pending {
		body.Items[i] = api.PendingBounce{
			ExternalDepositId: p.ExternalDepositID, CabalId: p.CabalID.UUID(), Status: p.Status, Amount: p.Amount,
			Mint: p.Mint, Attempts: p.Attempts, Since: p.Since.UTC(),
		}
		if p.ReturnAddress != "" {
			body.Items[i].ReturnAddress = &p.ReturnAddress
		}
	}
	return api.GetBounceQueue200JSONResponse(body), nil
}
