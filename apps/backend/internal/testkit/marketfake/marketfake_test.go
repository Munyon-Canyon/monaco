package marketfake_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestCatalogFake_findsAnAssetByIDMintOrSymbol(t *testing.T) {
	t.Parallel()
	f := marketfake.NewCatalog(marketfake.Fixtures()...)
	aapl := marketfake.AAPLx()
	for by, got := range map[string]func() (market.Asset, error){
		"id":     func() (market.Asset, error) { return f.AssetByID(t.Context(), aapl.ID) },
		"mint":   func() (market.Asset, error) { return f.AssetByMint(t.Context(), aapl.Mint) },
		"symbol": func() (market.Asset, error) { return f.AssetBySymbol(t.Context(), "AAPLx") },
	} {
		if a, err := got(); err != nil || a != aapl {
			t.Fatalf("by %s = %+v, %v", by, a, err)
		}
	}
	if _, err := f.AssetByMint(t.Context(), market.Mint{}); errs.CodeOf(err) != errs.CodeAssetNotFound {
		t.Fatalf("unknown mint err = %v, want asset_not_found", err)
	}
}

func TestCatalogFake_listsTradableAndAllAndFailsWhereFaultsSay(t *testing.T) {
	t.Parallel()
	f := marketfake.NewCatalog(marketfake.Fixtures()...)
	tradable, err := f.ListTradable(t.Context())
	halting := func(a market.Asset) bool { return !a.Tradable() }
	if err != nil || len(tradable) != 2 || slices.ContainsFunc(tradable, halting) {
		t.Fatalf("ListTradable = %v, %v, want AAPLx and TSLAx", tradable, err)
	}
	halted := marketfake.JPSTx()
	halted.IssuerTradable = true
	f.Put(halted)
	all, err := f.ListAll(t.Context())
	if err != nil || len(all) != 3 || !all[2].Tradable() {
		t.Fatalf("ListAll after Put = %v, %v, want the replaced JPSTx tradable", all, err)
	}
	f.FailOnce("ListAll", errs.New(errs.CodeDBUnavailable, "test"))
	if _, err := f.ListAll(t.Context()); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("ListAll err = %v, want the scripted fault", err)
	}
	f.FailOnce("AssetBySymbol", errs.New(errs.CodeDBUnavailable, "test"))
	if _, err := f.AssetBySymbol(t.Context(), "AAPLx"); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("AssetBySymbol err = %v, want the scripted fault", err)
	}
}
