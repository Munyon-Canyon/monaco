package treasury

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
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
	at := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	gotPrices, err := marketPrices(prices)(t.Context(), []uuid.UUID{asset.ID.UUID()}, at)
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
	prices.FailOnce("PricesAsOf", errs.New(errs.CodeDBUnavailable, "test"))
	if _, err := marketPrices(prices)(t.Context(), []uuid.UUID{asset.ID.UUID()}, at); err == nil {
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

type listingPricesFake struct {
	prices map[market.ListingKey]market.ListingPrice
	err    error
}

func (f listingPricesFake) ListingPrices(
	context.Context, []market.ListingKey, time.Time,
) (map[market.ListingKey]market.ListingPrice, error) {
	return f.prices, f.err
}

func TestMarketListingsCarriesTheListingItsStockAndItsPrice(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	priced := market.ListingPrice{Price: &market.Price{Micros: money.MicrosFromUint64(30), ObservedAt: at}}
	priced.Issuer, priced.Symbol, priced.Network, priced.Decimals = "ondo", "AAPLon", chain.NetworkBase, 18
	priced.SharesPerToken = big.NewRat(1, 1)
	priced.Asset = &market.ListedAsset{Mint: "XsbE", Symbol: "AAPLx", DisplayName: "Apple", Kind: "equity"}
	unpriced := market.ListingPrice{}
	got, err := marketListings(listingPricesFake{prices: map[market.ListingKey]market.ListingPrice{
		"base:0x1": priced, "base:0x2": unpriced,
	}})(t.Context(), []string{"base:0x1", "base:0x2"}, at)
	one := got["base:0x1"]
	if err != nil || one.Price == nil || one.Price.Micros.Uint64() != 30 || one.Asset == nil ||
		one.Asset.Symbol != "AAPLx" || one.Network != chain.NetworkBase ||
		got["base:0x2"].Price != nil || got["base:0x2"].Asset != nil {
		t.Fatalf("marketListings = %+v, %v", got, err)
	}
	if v, err := one.Value(big.NewInt(1_000_000_000_000_000_000)); err != nil || v.Uint64() != 30 {
		t.Fatalf("Value of one token = %v, %v, want 30", v, err)
	}
	down := errs.New(errs.CodeDBUnavailable, "test")
	if _, err := marketListings(
		listingPricesFake{err: down},
	)(
		t.Context(),
		nil,
		at,
	); errs.CodeOf(
		err,
	) != errs.CodeDBUnavailable {
		t.Fatalf("marketListings over a failed read = %v, want db_unavailable", err)
	}
}

func TestCatalogLogos_mapsSymbolsToTheirLogos(t *testing.T) {
	t.Parallel()
	aapl, spacex := marketfake.AAPLx(), marketfake.TSpaceX()
	catalog := marketfake.NewCatalog(aapl, spacex)
	got, err := catalogLogos{Catalog: catalog}.LogosBySymbol(t.Context())
	if err != nil || len(got) != 1 || got[aapl.Symbol] != aapl.LogoURL || aapl.LogoURL == "" {
		t.Fatalf("LogosBySymbol = %v, %v, want only %s", got, err, aapl.Symbol)
	}
	catalog.FailOnce("ListAll", errs.New(errs.CodeDBUnavailable, "test"))
	if _, err := (catalogLogos{Catalog: catalog}).LogosBySymbol(t.Context()); err == nil {
		t.Fatal("LogosBySymbol over a failed catalog = nil, want an error")
	}
}

func TestNew_refusesHeldCabalsUntilWired(t *testing.T) {
	t.Parallel()
	if New(module.Deps{}).Held() == nil {
		t.Fatal("Held() = nil")
	}
	_, err := New(module.Deps{}).heldCabals.CabalsOf(t.Context(), ids.UserID{})
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("CabalsOf before Wire = %v, want upstream_unavailable", err)
	}
}

type stubPositionPrices struct {
	points []market.PositionPoint
	err    error
}

func (s stubPositionPrices) PositionPrices(context.Context, string, string) ([]market.PositionPoint, error) {
	return s.points, s.err
}

func TestPositionPrices_mapsTheMarketPointsAndKeepsItsError(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	fail := errs.New(errs.CodeDBUnavailable, "test")
	got, err := positionPrices(stubPositionPrices{
		points: []market.PositionPoint{{At: at, PriceMicros: money.MicrosFromUint64(9)}}, err: fail,
	})(t.Context(), "AAPLx", "1W")
	if !errors.Is(err, fail) || len(got) != 1 || !got[0].At.Equal(at) || got[0].PriceMicros.Uint64() != 9 {
		t.Fatalf("positionPrices = %+v, %v, want one point of 9 and the market error", got, err)
	}
}
