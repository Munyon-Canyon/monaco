package market_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func insert(t *testing.T, pool *pgxpool.Pool, a market.Asset, override *bool) {
	t.Helper()
	var logo, rank, checked any
	if a.LogoURL != "" {
		logo = a.LogoURL
	}
	if a.PopularRank != 0 {
		rank = a.PopularRank
	}
	if a.ChainChecked {
		checked = a.UpdatedAt
	}
	_, err := pool.Exec(t.Context(), `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name,
		logo_url, issuer_tradable, tradable_override, popular_rank, company_key, first_seen_at, updated_at,
		chain_checked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		a.ID.UUID(), a.Symbol, a.Mint.String(), int16(a.Decimals), string(a.Issuer), string(a.Kind), a.DisplayName,
		logo, a.IssuerTradable, override, rank, a.CompanyKey, a.FirstSeenAt, a.UpdatedAt, checked)
	if err != nil {
		t.Fatal(err)
	}
}

func stamped(a market.Asset, at time.Time) market.Asset {
	a.FirstSeenAt, a.UpdatedAt = at, at
	return a
}

func seededCatalog(t *testing.T) (*app.Catalog, *pgxpool.Pool, time.Time) {
	t.Helper()
	pool := testkit.DB(t)
	at := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	for _, a := range marketfake.Fixtures() {
		insert(t, pool, stamped(a, at), nil)
	}
	return market.New(module.Deps{Pool: pool}).Catalog(), pool, at
}

func TestCatalog_readsAnAssetByIDMintAndSymbol(t *testing.T) {
	t.Parallel()
	catalog, _, at := seededCatalog(t)
	want := stamped(marketfake.AAPLx(), at)
	lookups := map[string]func(context.Context) (market.Asset, error){
		"id":     func(ctx context.Context) (market.Asset, error) { return catalog.AssetByID(ctx, want.ID) },
		"mint":   func(ctx context.Context) (market.Asset, error) { return catalog.AssetByMint(ctx, want.Mint) },
		"symbol": func(ctx context.Context) (market.Asset, error) { return catalog.AssetBySymbol(ctx, "AAPLx") },
	}
	for by, lookup := range lookups {
		got, err := lookup(t.Context())
		if err != nil {
			t.Fatalf("by %s: %v", by, err)
		}
		if !got.FirstSeenAt.Equal(at) || !got.UpdatedAt.Equal(at) {
			t.Fatalf("by %s: timestamps = %v, %v, want %v", by, got.FirstSeenAt, got.UpdatedAt, at)
		}
		got.FirstSeenAt, got.UpdatedAt = want.FirstSeenAt, want.UpdatedAt
		if got != want {
			t.Fatalf("by %s = %+v, want %+v", by, got, want)
		}
	}
}

func TestCatalog_AssetByMint_NotFound(t *testing.T) {
	t.Parallel()
	catalog, _, _ := seededCatalog(t)
	unknown, err := domain.ParseMint("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.AssetByMint(t.Context(), unknown); errs.CodeOf(err) != errs.CodeAssetNotFound {
		t.Fatalf("AssetByMint(unknown) err = %v, want asset_not_found", err)
	}
	if errs.KindOf(errs.CodeAssetNotFound) != errs.KindNotFound {
		t.Fatal("asset_not_found is not a not-found kind")
	}
	if _, err := catalog.AssetBySymbol(t.Context(), "NOPEx"); errs.CodeOf(err) != errs.CodeAssetNotFound {
		t.Fatalf("AssetBySymbol(unknown) err = %v, want asset_not_found", err)
	}
}

func symbols(assets []market.Asset) []string {
	out := make([]string, len(assets))
	for i, a := range assets {
		out[i] = a.Symbol
	}
	return out
}

func TestCatalog_listsTradablePopularFirstAndAllBySymbol(t *testing.T) {
	t.Parallel()
	catalog, pool, at := seededCatalog(t)
	unranked := stamped(marketfake.TSLAx(), at)
	unranked.ID = domain.NewAssetID(testkit.NewIDs(1))
	unranked.Symbol, unranked.PopularRank, unranked.IssuerTradable = "ABCx", 0, false
	unranked.Mint, _ = domain.ParseMint("So11111111111111111111111111111111111111112")
	on, off := true, false
	insert(t, pool, unranked, &on)
	halted := stamped(marketfake.TSLAx(), at)
	halted.ID = domain.NewAssetID(testkit.NewIDs(2))
	halted.Symbol, halted.PopularRank = "OFFx", 0
	halted.Mint, _ = domain.ParseMint("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	insert(t, pool, halted, &off)

	tradable, err := catalog.ListTradable(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got := symbols(tradable); !slices.Equal(got, []string{"AAPLx", "TSLAx", "ABCx"}) {
		t.Fatalf("ListTradable = %v, want AAPLx, TSLAx, then ABCx by its override", got)
	}
	priceable, err := catalog.ListPriceable(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	got := symbols(priceable)
	slices.Sort(got)
	if !slices.Equal(got, []string{"AAPLx", "ABCx", "TSLAx"}) {
		t.Fatalf("ListPriceable = %v, want the same checked, tradable set as ListTradable", got)
	}
	all, err := catalog.ListAll(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got := symbols(all); !slices.Equal(got, []string{"AAPLx", "ABCx", "JPSTx", "OFFx", "TSLAx"}) {
		t.Fatalf("ListAll = %v, want every asset by symbol", got)
	}
	for _, a := range all {
		if listed := slices.Contains(symbols(tradable), a.Symbol); listed != a.Tradable() {
			t.Fatalf("%s: listed tradable %v, but Tradable() = %v", a.Symbol, listed, a.Tradable())
		}
	}
	if all[1].Override != domain.OverrideOn || all[3].Override != domain.OverrideOff {
		t.Fatalf("overrides ABCx=%s OFFx=%s, want on and off", all[1].Override, all[3].Override)
	}
}

func TestCatalog_uncheckedAssetIsReadableButNeverListedTradable(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	catalog := market.New(module.Deps{Pool: pool}).Catalog()
	at := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	unchecked := stamped(marketfake.AAPLx(), at)
	unchecked.ChainChecked = false
	on := true
	insert(t, pool, unchecked, &on)
	insert(t, pool, stamped(marketfake.TSLAx(), at), nil)
	tradable, err := catalog.ListTradable(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got := symbols(tradable); !slices.Equal(got, []string{"TSLAx"}) {
		t.Fatalf("ListTradable = %v, want only TSLAx while AAPLx's chain facts are unchecked", got)
	}
	got, err := catalog.AssetByMint(t.Context(), unchecked.Mint)
	if err != nil || got.Symbol != "AAPLx" || got.ChainChecked || got.Tradable() {
		t.Fatalf("AssetByMint(AAPLx) = %+v, %v, want the unchecked, untradable asset", got, err)
	}
}

func TestCatalog_rowThatDoesNotParseIsADecodeFailure(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	catalog := market.New(module.Deps{Pool: pool}).Catalog()
	a := marketfake.AAPLx()
	_, err := pool.Exec(t.Context(), `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name,
		issuer_tradable, company_key, first_seen_at, updated_at)
		VALUES ($1, 'AAPLx', 'not-a-mint', 300, 'xstocks', 'equity', 'Apple xStock', true, 'apple', now(), now())`,
		a.ID.UUID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.AssetBySymbol(t.Context(), "AAPLx"); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("AssetBySymbol err = %v, want decode_failed", err)
	}
	if _, err := catalog.ListAll(t.Context()); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("ListAll err = %v, want decode_failed", err)
	}
}

func TestCatalog_databaseErrorKeepsItsCode(t *testing.T) {
	t.Parallel()
	catalog, _, _ := seededCatalog(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := catalog.AssetByID(ctx, marketfake.AAPLx().ID)
	if err == nil || errs.CodeOf(err) == errs.CodeAssetNotFound {
		t.Fatalf("AssetByID on a cancelled context err = %v, want a database error", err)
	}
	if _, err := catalog.ListTradable(ctx); err == nil {
		t.Fatal("ListTradable on a cancelled context succeeded")
	}
}

func TestSiblings_SpaceX(t *testing.T) {
	t.Parallel()
	catalog, pool, at := seededCatalog(t)
	tessera, prestocks := stamped(marketfake.TSpaceX(), at), stamped(marketfake.SPACEX(), at)
	insert(t, pool, tessera, nil)
	insert(t, pool, prestocks, nil)
	for _, tc := range []struct{ of, want market.Asset }{{tessera, prestocks}, {prestocks, tessera}} {
		got, err := catalog.Siblings(t.Context(), tc.of.ID)
		if err != nil || len(got) != 1 {
			t.Fatalf("Siblings(%s) = %+v, %v, want only %s", tc.of.Symbol, got, err, tc.want.Symbol)
		}
		got[0].FirstSeenAt, got[0].UpdatedAt = tc.want.FirstSeenAt, tc.want.UpdatedAt
		if got[0] != tc.want {
			t.Fatalf("Siblings(%s) = %+v, want %+v", tc.of.Symbol, got[0], tc.want)
		}
	}
	if got, err := catalog.Siblings(t.Context(), marketfake.AAPLx().ID); err != nil || len(got) != 0 {
		t.Fatalf("Siblings(AAPLx) = %+v, %v, want none", got, err)
	}
	missing := domain.NewAssetID(testkit.NewIDs(99))
	if _, err := catalog.Siblings(t.Context(), missing); errs.CodeOf(err) != errs.CodeAssetNotFound {
		t.Fatalf("Siblings(unknown) err = %v, want asset_not_found", err)
	}
}

func TestSiblings_ordersByIssuer(t *testing.T) {
	t.Parallel()
	catalog, pool, at := seededCatalog(t)
	tessera, prestocks := stamped(marketfake.TSpaceX(), at), stamped(marketfake.SPACEX(), at)
	insert(t, pool, tessera, nil)
	insert(t, pool, prestocks, nil)
	xstock := stamped(marketfake.AAPLx(), at)
	_, err := pool.Exec(t.Context(), `UPDATE assets SET company_key = 'spacex' WHERE id = $1`, xstock.ID.UUID())
	if err != nil {
		t.Fatal(err)
	}
	got, err := catalog.Siblings(t.Context(), xstock.ID)
	if err != nil || len(got) != 2 || got[0].Symbol != "SPACEX" || got[1].Symbol != "tSpaceX" {
		t.Fatalf("Siblings(AAPLx) = %+v, %v, want SPACEX (prestocks) then tSpaceX (tessera)", got, err)
	}
}
