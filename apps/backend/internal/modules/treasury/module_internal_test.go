package treasury

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestMarketReadersMapCatalogAndPrices(t *testing.T) {
	t.Parallel()
	asset := marketfake.AAPLx()
	catalog := marketfake.NewCatalog(asset)
	got, err := marketResolver(catalog)(t.Context(), asset.Mint.Address())
	if err != nil || got.ID != asset.ID.UUID() || got.Decimals != asset.Decimals {
		t.Fatalf("marketResolver() = %#v, %v", got, err)
	}
	prices := &marketfake.PricesFake{}
	prices.Set(asset.ID, money.MicrosFromUint64(7), time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC))
	gotPrices, err := marketPrices(prices)(t.Context())
	if err != nil || gotPrices[asset.ID.UUID()].Micros.Uint64() != 7 {
		t.Fatalf("marketPrices() = %#v, %v", gotPrices, err)
	}
	if _, err := market.ParseMint("bad"); err == nil {
		t.Fatal("ParseMint error = nil")
	}
	if _, err := marketResolver(catalog)(t.Context(), "bad"); err == nil {
		t.Fatal("marketResolver invalid mint error = nil")
	}
	catalog.FailOnce("AssetByMint", errs.New(errs.CodeDBUnavailable, "test"))
	if _, err := marketResolver(catalog)(t.Context(), asset.Mint.Address()); err == nil {
		t.Fatal("marketResolver error = nil")
	}
	prices.FailOnce("LatestPrices", errs.New(errs.CodeDBUnavailable, "test"))
	if _, err := marketPrices(prices)(t.Context()); err == nil {
		t.Fatal("marketPrices error = nil")
	}
}

func TestMarketResolverCarriesCatalogNames(t *testing.T) {
	t.Parallel()
	asset := marketfake.AAPLx()
	got, err := marketResolver(marketfake.NewCatalog(asset))(t.Context(), asset.Mint.Address())
	if err != nil || got.Symbol != asset.Symbol || got.DisplayName != asset.DisplayName || got.Symbol == "" {
		t.Fatalf("marketResolver() names = %q %q, %v", got.Symbol, got.DisplayName, err)
	}
}
