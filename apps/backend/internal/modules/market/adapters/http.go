package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

var _ httpx.MarketRoutes = HTTP{}

type HTTP struct{}

func (HTTP) GetAssets(ctx context.Context, _ api.GetAssetsRequestObject) (api.GetAssetsResponseObject, error) {
	if _, err := caller(ctx); err != nil {
		return nil, err
	}
	return api.GetAssets200JSONResponse{Assets: []api.AssetSummary{}}, nil
}

func caller(ctx context.Context) (auth.Actor, error) {
	actor, ok := auth.ActorFrom(ctx)
	if !ok {
		return auth.Actor{}, errs.New(errs.CodeUnauthorized, "market.caller")
	}
	if actor.Kind != auth.ActorUser {
		return auth.Actor{}, errs.New(errs.CodeForbidden, "market.caller")
	}
	if _, err := ids.ParseUserID(actor.ID); err != nil {
		return auth.Actor{}, errs.Wrap(err, errs.CodeUnauthorized, "market.caller")
	}
	return actor, nil
}
