package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
)

const (
	opTrade        = "notify.Trade"
	proposalSource = "proposal"
	unnamedAsset   = "a stock"
)

type Assets interface {
	AssetBySymbol(ctx context.Context, symbol string) (market.Asset, error)
}

func assetOf(ctx context.Context, assets Assets, symbol string) (market.Asset, bool, error) {
	asset, err := assets.AssetBySymbol(ctx, symbol)
	switch {
	case err == nil:
		return asset, true, nil
	case errs.CodeOf(err) == errs.CodeAssetNotFound:
		return market.Asset{}, false, nil
	}
	return market.Asset{}, false, err
}

func labelOf(ctx context.Context, assets Assets, symbol string) (string, error) {
	asset, found, err := assetOf(ctx, assets, symbol)
	if err != nil || !found {
		return unnamedAsset, err
	}
	return assetLabel(asset), nil
}
