package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
)

type AssetLookup interface {
	AssetByID(ctx context.Context, id domain.AssetID) (domain.Asset, error)
}

type Calendar struct {
	assets AssetLookup
}

func NewCalendar(assets AssetLookup) *Calendar { return &Calendar{assets: assets} }

func (c *Calendar) Session(ctx context.Context, id domain.AssetID, at time.Time) (domain.SessionInfo, error) {
	asset, err := c.assets.AssetByID(ctx, id)
	if err != nil {
		return domain.SessionInfo{}, err
	}
	return domain.Session(asset.Kind, at)
}
