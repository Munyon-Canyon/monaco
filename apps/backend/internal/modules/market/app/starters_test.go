package app

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func starterMint(i int) string {
	return []string{
		"4vJ9JU1bJJE96FWSJKvHsmmFADCg4gpZQff4P3bkLKi",
		"8qbHbw2BbbTHBW1sbeqakYXVKRQM8Ne7pLK7m6CVfeR",
		"CktRuQ2mttgRGkXJtyksdKHjUdc2C4TgDzyB98oEzy8",
		"GgBaCs3NCBuZN12kCJgAW63ydqohFkHEdfdEXBPzLHq",
		"LbUiWL3xVV8hTFYBVdbTNrpDo41NKS6o3LHHuDzjfcY",
		"QWmroo4YnnMqYW3cnxWkFdaTxGD3P7vMSzwMHGbUzwF",
		"US517G5965aydkZ46HS38QLi7UQiSojurfbQfKCELFx",
		"YMN9Qj5jPNp7j14VPcML1B6xGgcPWVZUGLFU3Mnyfaf",
		"cGfHiC6Kgg3FpFZvgwGcswsCRtp4aBP2fzuXRQPizuN",
	}[i]
}

type starterSet struct {
	assets []domain.Asset
	byName map[string]domain.Asset
}

func (s starterSet) held(name string, cabals int) HolderCount {
	return HolderCount{Mint: s.byName[name].Mint.Address(), Cabals: cabals}
}

func newStarterSet(t *testing.T, specs ...starterSpec) starterSet {
	t.Helper()
	set := starterSet{byName: map[string]domain.Asset{}}
	g := testkit.NewIDs(7)
	for i, spec := range specs {
		row := appleRow(t, "equity")
		row.ID = g.NewV7()
		row.Symbol, row.DisplayName, row.Mint = spec.symbol, spec.symbol, starterMint(i)
		asset, err := toAsset(row)
		if err != nil {
			t.Fatal(err)
		}
		asset.ChainChecked, asset.PopularRank, asset.IssuerTradable = true, spec.rank, !spec.untradable
		set.assets = append(set.assets, asset)
		set.byName[spec.symbol] = asset
	}
	return set
}

type starterSpec struct {
	symbol     string
	rank       int16
	untradable bool
}

type starterRig struct {
	*ListAssets
	clock *testkit.Clock
	calls *int
}

func (s starterSet) rig(t *testing.T, counts ...HolderCount) starterRig {
	t.Helper()
	calls := 0
	clk := testkit.NewClock(time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC))
	l := &ListAssets{read: reader{}, volumes: reader{}, clock: clk, catalog: fixedAssets{assets: s.assets}}
	l.WithHolders(func(context.Context) ([]HolderCount, error) {
		calls++
		return counts, nil
	})
	return starterRig{ListAssets: l, clock: clk, calls: &calls}
}

type starterRow struct {
	symbol string
	count  int
}

func starterRows(t *testing.T, l *ListAssets, req ListRequest) []starterRow {
	t.Helper()
	req.Filter = FilterInCabals
	page, err := l.Handle(t.Context(), req)
	if err != nil || page.NextCursor != "" {
		t.Fatalf("Handle = %+v, %v", page, err)
	}
	out := make([]starterRow, len(page.Items))
	for i, item := range page.Items {
		out[i] = starterRow{symbol: item.Asset.Symbol}
		if item.CabalCount != nil {
			out[i].count = *item.CabalCount
		}
	}
	return out
}

func TestStarters_ordersByCountThenPopularRankThenSymbol(t *testing.T) {
	t.Parallel()
	set := newStarterSet(t,
		starterSpec{symbol: "AAA", rank: 3}, starterSpec{symbol: "BBB", rank: 1},
		starterSpec{symbol: "CCC"}, starterSpec{symbol: "DDD"}, starterSpec{symbol: "EEE"},
	)
	l := set.rig(t, set.held("EEE", 5), set.held("AAA", 5), set.held("DDD", 5), set.held("BBB", 5), set.held("CCC", 9))
	got := starterRows(t, l.ListAssets, ListRequest{Limit: 5})
	want := []starterRow{{"CCC", 9}, {"BBB", 5}, {"AAA", 5}, {"DDD", 5}, {"EEE", 5}}
	if !slices.Equal(got, want) {
		t.Fatalf("rows = %+v, want %+v", got, want)
	}
}

func TestStarters_omitsAnAssetHeldByFewerThanThreeCabals(t *testing.T) {
	t.Parallel()
	set := newStarterSet(t, starterSpec{symbol: "AAA"}, starterSpec{symbol: "BBB"}, starterSpec{symbol: "POP", rank: 1})
	l := set.rig(t, set.held("AAA", 2), set.held("BBB", 3), set.held("POP", 1))
	got := starterRows(t, l.ListAssets, ListRequest{})
	want := []starterRow{{"BBB", 3}, {"POP", 0}}
	if !slices.Equal(got, want) {
		t.Fatalf("rows = %+v, want %+v (AAA has two cabals; POP drops its count of one)", got, want)
	}
}

func TestStarters_appliesTheLimitAfterEligibility(t *testing.T) {
	t.Parallel()
	set := newStarterSet(t,
		starterSpec{symbol: "U1", untradable: true}, starterSpec{symbol: "U2", untradable: true},
		starterSpec{symbol: "U3", untradable: true}, starterSpec{symbol: "T1"}, starterSpec{symbol: "T2"},
		starterSpec{symbol: "T3"}, starterSpec{symbol: "T4"}, starterSpec{symbol: "POP1", rank: 1},
		starterSpec{symbol: "POP2", rank: 2},
	)
	l := set.rig(t,
		set.held("U1", 90), set.held("U2", 80), set.held("U3", 70),
		set.held("T1", 60), set.held("T2", 50), set.held("T3", 40), set.held("T4", 30),
	)
	got := starterRows(t, l.ListAssets, ListRequest{Limit: 5})
	want := []starterRow{{"T1", 60}, {"T2", 50}, {"T3", 40}, {"T4", 30}, {"POP1", 0}}
	if !slices.Equal(got, want) {
		t.Fatalf("rows = %+v, want %+v", got, want)
	}
}

func TestStarters_listsACountedPopularAssetOnceWithItsCount(t *testing.T) {
	t.Parallel()
	set := newStarterSet(t, starterSpec{symbol: "POP1", rank: 1}, starterSpec{symbol: "POP2", rank: 2})
	l := set.rig(t, set.held("POP2", 4))
	got := starterRows(t, l.ListAssets, ListRequest{Limit: 5})
	want := []starterRow{{"POP2", 4}, {"POP1", 0}}
	if !slices.Equal(got, want) {
		t.Fatalf("rows = %+v, want %+v", got, want)
	}
}

func TestStarters_fillsFromPopularWhenNothingIsCounted(t *testing.T) {
	t.Parallel()
	set := newStarterSet(t,
		starterSpec{symbol: "OTHER"}, starterSpec{symbol: "POP2", rank: 2}, starterSpec{symbol: "POP1", rank: 1},
		starterSpec{symbol: "GONE", rank: 3, untradable: true},
	)
	for name, l := range map[string]starterRig{"no holders": set.rig(t), "holders unwired": set.rig(t)} {
		if name == "holders unwired" {
			l.holders = nil
		}
		got := starterRows(t, l.ListAssets, ListRequest{})
		want := []starterRow{{"POP1", 0}, {"POP2", 0}}
		if !slices.Equal(got, want) {
			t.Fatalf("%s: rows = %+v, want %+v", name, got, want)
		}
	}
}

func TestStarters_servesASecondRequestFromTheCacheForFiveMinutes(t *testing.T) {
	t.Parallel()
	set := newStarterSet(t, starterSpec{symbol: "AAA"})
	l := set.rig(t, set.held("AAA", 3))
	for range 2 {
		starterRows(t, l.ListAssets, ListRequest{})
	}
	if *l.calls != 1 {
		t.Fatalf("port calls after two requests = %d, want 1", *l.calls)
	}
	l.clock.Advance(starterTTL - time.Second)
	starterRows(t, l.ListAssets, ListRequest{})
	if *l.calls != 1 {
		t.Fatalf("port calls just inside the ttl = %d, want 1", *l.calls)
	}
	l.clock.Advance(time.Second)
	starterRows(t, l.ListAssets, ListRequest{})
	if *l.calls != 2 {
		t.Fatalf("port calls after the ttl = %d, want 2", *l.calls)
	}
}

func TestStarters_searchNarrowsBothTheCountedAndTheFilledRows(t *testing.T) {
	t.Parallel()
	set := newStarterSet(t,
		starterSpec{symbol: "APPL"}, starterSpec{symbol: "AMD", rank: 1}, starterSpec{symbol: "NVDA", rank: 2},
	)
	l := set.rig(t, set.held("APPL", 3), set.held("NVDA", 4))
	for q, want := range map[string][]starterRow{"ap": {{"APPL", 3}}, "am": {{"AMD", 0}}} {
		if got := starterRows(t, l.ListAssets, ListRequest{Q: q}); !slices.Equal(got, want) {
			t.Fatalf("q=%q: rows = %+v, want %+v", q, got, want)
		}
	}
}

func TestStarters_failsWhenTheHolderCountOrTheCatalogFails(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "market.test")
	clk := testkit.NewClock(time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC))
	counts := func(context.Context) ([]HolderCount, error) { return nil, boom }
	failing := &ListAssets{read: reader{}, volumes: reader{}, clock: clk, catalog: fixedAssets{}}
	set := newStarterSet(t, starterSpec{symbol: "AAA", rank: 1})
	summaries := set.rig(t)
	summaries.read = reader{newest: func() ([]sqlc.NewestSamplesRow, error) { return nil, boom }}
	cases := map[string]*ListAssets{
		"summaries": summaries.ListAssets,
		"holders":   failing.WithHolders(counts),
		"catalog":   {read: reader{}, volumes: reader{}, clock: clk, catalog: fixedAssets{err: boom}},
	}
	for name, l := range cases {
		if _, err := l.Handle(t.Context(), ListRequest{Filter: FilterInCabals}); errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("%s = %v, want internal", name, err)
		}
	}
}

func TestStarters_defaultsToFiveRowsAndTakesNoCursor(t *testing.T) {
	t.Parallel()
	got, err := normalize(ListRequest{Filter: FilterInCabals})
	if err != nil || got.limit != starterDefault {
		t.Fatalf("normalize(in_cabals) = %+v, %v, want limit %d", got, err, starterDefault)
	}
	got, err = normalize(ListRequest{Filter: FilterInCabals, Limit: 12})
	if err != nil || got.limit != 12 {
		t.Fatalf("normalize(in_cabals, 12) = %+v, %v, want limit 12", got, err)
	}
	for name, req := range map[string]ListRequest{
		"cursor":      {Filter: FilterInCabals, Cursor: "c0FBUEx4"},
		"limit of 51": {Filter: FilterInCabals, Limit: 51},
	} {
		if _, err = normalize(req); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("%s on in_cabals = %v, want invalid_input", name, err)
		}
	}
}
