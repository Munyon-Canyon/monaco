package marketfake

import (
	"cmp"
	"context"
	"log/slog"
	"slices"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

var (
	_ market.Catalog         = (*CatalogFake)(nil)
	_ market.CompanyListings = (*CatalogFake)(nil)
)

type CatalogFake struct {
	testkit.Faults

	mu     sync.Mutex
	assets []market.Asset
}

func NewCatalog(assets ...market.Asset) *CatalogFake {
	f := &CatalogFake{}
	for _, a := range assets {
		f.Put(a)
	}
	return f
}

func Fixtures() []market.Asset { return []market.Asset{AAPLx(), TSLAx(), JPSTx()} }

func AAPLx() market.Asset {
	return fixture("01920000-0000-7000-8000-000000000001", "AAPLx", "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp",
		"Apple", 1, true)
}

func TSLAx() market.Asset {
	return fixture("01920000-0000-7000-8000-000000000002", "TSLAx", "XsDoVfqeBukxuZHWhdvWHBhgEHjGNst4MLodqsJHzoB",
		"Tesla", 7, true)
}

func JPSTx() market.Asset {
	return fixture("01920000-0000-7000-8000-000000000003", "JPSTx", "XsCAXu7xTaZMG9b9KJhNWYapuvNjxPuE4SysZq8uvMq",
		"JPMorgan Ultra-Short Income", 0, false)
}

func TSpaceX() market.Asset {
	return preIPO("01920000-0000-7000-8000-000000000004", "tSpaceX", "TSPXcLV76s6V2zDiZQ18kBfcbnjaE2ZzNT3ga2Pd99v",
		domain.IssuerTessera)
}

func SPACEX() market.Asset {
	return preIPO("01920000-0000-7000-8000-000000000005", "SPACEX", "PreANxuXjsy2pvisWWMNB6YaJNzr7681wJJr2rHsfTh",
		domain.IssuerPreStocks)
}

func preIPO(id, symbol, mint string, issuer domain.Issuer) market.Asset {
	a := fixture(id, symbol, mint, "SpaceX", 0, true)
	a.Decimals, a.Issuer, a.Kind, a.LogoURL = 9, issuer, domain.KindPreIPO, ""
	return a
}

func fixture(id, symbol, mint, name string, rank int16, tradable bool) market.Asset {
	assetID, err := domain.ParseAssetID(id)
	if err != nil {
		panic(err)
	}
	m, err := domain.ParseMint(mint)
	if err != nil {
		panic(err)
	}
	return market.Asset{
		ID: assetID, Symbol: symbol, Mint: m, Decimals: 8, Issuer: domain.IssuerXStocks, Kind: domain.KindEquity,
		DisplayName: name, LogoURL: "https://xstocks-metadata.backed.fi/logos/tokens/" + symbol + ".png",
		UIMultiplier: domain.Multiplier{Num: 1, Den: 1}, ChainChecked: true, IssuerTradable: tradable,
		Override: domain.OverrideAuto, PopularRank: rank, CompanyKey: domain.CompanyKey(domain.IssuerXStocks, name),
	}
}

func (f *CatalogFake) Put(a market.Asset) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i := slices.IndexFunc(f.assets, func(b market.Asset) bool { return b.ID == a.ID }); i >= 0 {
		f.assets[i] = a
		return
	}
	f.assets = append(f.assets, a)
}

func (f *CatalogFake) AssetByID(_ context.Context, id market.AssetID) (market.Asset, error) {
	return f.find("AssetByID", func(a market.Asset) bool { return a.ID == id }, slog.String("id", id.String()))
}

func (f *CatalogFake) AssetByMint(_ context.Context, mint market.Mint) (market.Asset, error) {
	match := func(a market.Asset) bool { return a.Mint == mint }
	return f.find("AssetByMint", match, slog.String("mint", mint.String()))
}

func (f *CatalogFake) AssetBySymbol(_ context.Context, symbol string) (market.Asset, error) {
	match := func(a market.Asset) bool { return a.Symbol == symbol }
	return f.find("AssetBySymbol", match, slog.String("symbol", symbol))
}

func (f *CatalogFake) ListTradable(_ context.Context) ([]market.Asset, error) {
	return f.list("ListTradable", market.Asset.Tradable)
}

func (f *CatalogFake) ListAll(_ context.Context) ([]market.Asset, error) {
	return f.list("ListAll", func(market.Asset) bool { return true })
}

func (f *CatalogFake) Siblings(ctx context.Context, id market.AssetID) ([]market.Asset, error) {
	asset, err := f.AssetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	out, err := f.list("Siblings", func(a market.Asset) bool { return a.CompanyKey == asset.CompanyKey && a.ID != id })
	slices.SortFunc(out, func(a, b market.Asset) int {
		return cmp.Or(cmp.Compare(a.Issuer, b.Issuer), cmp.Compare(a.Symbol, b.Symbol))
	})
	return out, err
}

func (f *CatalogFake) find(op string, match func(market.Asset) bool, key slog.Attr) (market.Asset, error) {
	if err := f.Check(op); err != nil {
		return market.Asset{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if i := slices.IndexFunc(f.assets, match); i >= 0 {
		return f.assets[i], nil
	}
	return market.Asset{}, errs.New(errs.CodeAssetNotFound, "marketfake."+op, key)
}

func (f *CatalogFake) list(op string, keep func(market.Asset) bool) ([]market.Asset, error) {
	if err := f.Check(op); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []market.Asset{}
	for _, a := range f.assets {
		if keep(a) {
			out = append(out, a)
		}
	}
	return out, nil
}
