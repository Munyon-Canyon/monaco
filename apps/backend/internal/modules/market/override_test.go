package market_test

import (
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
)

func TestTradableOverride_WinsOverIssuerInTheCatalog(t *testing.T) {
	t.Parallel()
	catalog, pool, at := seededCatalog(t)
	later := at.Add(time.Minute)
	off, err := app.SetTradableOverride(t.Context(), pool, later, "AAPLx", domain.OverrideOff)
	if err != nil || off.Override != domain.OverrideOff || off.Tradable() || !off.IssuerTradable ||
		!off.UpdatedAt.Equal(later) {
		t.Fatalf("AAPLx off = %+v, %v, want untradable over a tradable issuer, updated %v", off, err, later)
	}
	on, err := app.SetTradableOverride(t.Context(), pool, later, "JPSTx", domain.OverrideOn)
	if err != nil || !on.Tradable() || on.IssuerTradable {
		t.Fatalf("JPSTx on = %+v, %v, want tradable over a halted issuer", on, err)
	}
	tradable, err := catalog.ListTradable(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got := symbols(tradable); !slices.Equal(got, []string{"TSLAx", "JPSTx"}) {
		t.Fatalf("ListTradable = %v, want TSLAx and JPSTx after the overrides", got)
	}
}

func TestTradableOverride_AutoClears(t *testing.T) {
	t.Parallel()
	catalog, pool, at := seededCatalog(t)
	for _, o := range []domain.Override{domain.OverrideOff, domain.OverrideAuto} {
		if _, err := app.SetTradableOverride(t.Context(), pool, at, "AAPLx", o); err != nil {
			t.Fatal(err)
		}
	}
	var stored *bool
	if err := pool.QueryRow(t.Context(), "SELECT tradable_override FROM assets WHERE symbol = 'AAPLx'").
		Scan(&stored); err != nil {
		t.Fatal(err)
	}
	a, err := catalog.AssetBySymbol(t.Context(), "AAPLx")
	if err != nil || stored != nil || a.Override != domain.OverrideAuto || !a.Tradable() {
		t.Fatalf(
			"AAPLx after auto = %+v (stored %v), %v, want the override cleared and the issuer followed",
			a,
			stored,
			err,
		)
	}
	if _, err := app.SetTradableOverride(t.Context(), pool, at, "NOPEx", domain.OverrideOn); errs.CodeOf(err) !=
		errs.CodeAssetNotFound {
		t.Fatalf("unknown symbol err = %v, want asset_not_found", err)
	}
}
