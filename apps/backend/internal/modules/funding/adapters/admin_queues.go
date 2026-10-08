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
	pending, err := app.BounceQueue(ctx, h.Reads, limit)
	if err != nil {
		return nil, err
	}
	body := api.BounceQueue{Items: make([]api.PendingBounce, len(pending))}
	for i, p := range pending {
		body.Items[i] = api.PendingBounce{CabalId: p.CabalID.UUID(), Since: p.Since.UTC()}
	}
	return api.GetBounceQueue200JSONResponse(body), nil
}
