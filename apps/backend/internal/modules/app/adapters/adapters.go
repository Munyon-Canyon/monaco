package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/app/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/appapi"
)

type HTTP struct {
	Config app.GetConfig
}

var _ api.StrictServerInterface = HTTP{}

func (h HTTP) GetAppConfig(context.Context, api.GetAppConfigRequestObject) (api.GetAppConfigResponseObject, error) {
	cfg := h.Config.Handle()
	return api.GetAppConfig200JSONResponse(api.AppConfig{MinIosBuild: cfg.MinIOSBuild, StoreUrl: cfg.StoreURL}), nil
}
