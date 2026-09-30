package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
)

type ProviderAsset struct {
	Symbol      string
	Mint        domain.Mint
	Decimals    uint8
	Kind        domain.Kind
	DisplayName string
	LogoURL     string
	Tradable    bool
}

type AssetProvider interface {
	Issuer() domain.Issuer
	Catalog(ctx context.Context) ([]ProviderAsset, error)
}
