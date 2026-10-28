package market_test

import (
	"context"
	"crypto/sha256"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type sampleCounter struct {
	newest    atomic.Int64
	sparkline atomic.Int64
}

func (c *sampleCounter) TraceQueryStart(
	ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData,
) context.Context {
	switch {
	case strings.Contains(data.SQL, "date_bin"):
		c.sparkline.Add(1)
	case strings.Contains(data.SQL, "LIMIT 3"):
		c.newest.Add(1)
	}
	return ctx
}

func (*sampleCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (s marketAPI) seedNamed(t *testing.T, count int) []string {
	t.Helper()
	g := testkit.NewIDs(6485)
	names := make([]string, count)
	for i, asset := range generatedCatalog(t, count) {
		sum := sha256.Sum256([]byte("name-" + strconv.Itoa(i)))
		var name strings.Builder
		for _, b := range sum[:7] {
			letter := rune('a' + b%26)
			if b&0x80 != 0 {
				letter -= 'a' - 'A'
			}
			name.WriteRune(letter)
		}
		asset.ID = domain.NewAssetID(g)
		asset.DisplayName, asset.ChainChecked = name.String(), true
		insert(t, s.pool, stamped(asset, s.when), nil)
		names[i] = asset.DisplayName
	}
	return names
}

func (s marketAPI) listAll(t *testing.T, l *app.ListAssets, sort app.Sort, limit int) []app.Summary {
	t.Helper()
	var out []app.Summary
	cursor := ""
	for range 200 {
		page, err := l.Handle(t.Context(), app.ListRequest{
			Filter: app.FilterAll, Sort: sort, Limit: limit, Cursor: cursor,
		})
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, page.Items...)
		if cursor = page.NextCursor; cursor == "" {
			return out
		}
	}
	t.Fatal("list never ended")
	return nil
}

func displayNames(items []app.Summary) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Asset.DisplayName
	}
	return out
}

func TestListOrder_nameOrdersAFullCatalogAcrossPages(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	names := s.seedNamed(t, 120)
	got := displayNames(s.listAll(t, app.NewListAssets(s.pool, s.clock), app.SortName, 25))
	want := slices.SortedFunc(slices.Values(names), func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})
	if len(got) != 120 || !slices.Equal(got, want) {
		t.Fatalf("sort=name = %d names %v, want 120 names A to Z ignoring case %v", len(got), got, want)
	}
}

func TestListOrder_nameKeepsSearchAndIsServedOnTheRoute(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	page := pageOf(t, s.get(t, "/v1/assets?filter=all&sort=name&limit=1"))
	if len(page.Assets) != 1 || page.NextCursor == nil || page.Assets[0].DisplayName != "Apple" {
		t.Fatalf("sort=name over http = %+v", page)
	}
	next := pageOf(t, s.get(t, "/v1/assets?filter=all&sort=name&limit=1&cursor="+*page.NextCursor))
	if len(next.Assets) != 1 || next.Assets[0].DisplayName != "Tesla" || next.NextCursor != nil {
		t.Fatalf("second page = %+v", next)
	}
	only := s.listAllQ(t, "apple")
	if len(only) != 1 || !strings.Contains(strings.ToLower(only[0].Asset.DisplayName), "apple") {
		t.Fatalf("q=apple sort=name = %v", displayNames(only))
	}
}

func (s marketAPI) listAllQ(t *testing.T, q string) []app.Summary {
	t.Helper()
	page, err := app.NewListAssets(s.pool, s.clock).Handle(t.Context(),
		app.ListRequest{Q: q, Filter: app.FilterAll, Sort: app.SortName})
	if err != nil {
		t.Fatal(err)
	}
	return page.Items
}

func (s marketAPI) seedPriced(t *testing.T) {
	t.Helper()
	g := testkit.NewIDs(6486)
	prices := []int64{0, 30_000_000, 250_000_000, 0, 30_000_000, 99_000_000, 0, 7_000_000, 410_000_000, 120_000_000}
	for i, asset := range generatedCatalog(t, len(prices)) {
		asset.ID = domain.NewAssetID(g)
		asset.ChainChecked = true
		insert(t, s.pool, stamped(asset, s.when), nil)
		if prices[i] != 0 {
			s.price(t, asset.Mint.String(), -time.Minute, prices[i])
		}
	}
}

func TestListOrder_priceIsHighestFirstThenUnpricedBySymbolEachOnce(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedPriced(t)
	want := []string{"G1008x", "G1002x", "G1009x", "G1005x", "G1001x", "G1004x", "G1007x", "G1000x", "G1003x", "G1006x"}
	for _, limit := range []int{3, 4, 50} {
		items := s.listAll(t, app.NewListAssets(s.pool, s.clock), app.SortPrice, limit)
		got := make([]string, len(items))
		for i, item := range items {
			got[i] = item.Asset.Symbol
		}
		if !slices.Equal(got, want) {
			t.Fatalf("sort=price limit %d = %v, want %v", limit, got, want)
		}
	}
}

func TestListOrder_priceNarrowsBySearchAndIsServedOnTheRoute(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedPriced(t)
	page := pageOf(t, s.get(t, "/v1/assets?filter=all&sort=price&limit=2&q=G100"))
	first, second := page.Assets[0].Symbol, page.Assets[1].Symbol
	if len(page.Assets) != 2 || first != "G1008x" || second != "G1002x" || page.NextCursor == nil {
		t.Fatalf("sort=price over http = %+v", page)
	}
	next := pageOf(t, s.get(t, "/v1/assets?filter=all&sort=price&limit=2&q=G100&cursor="+*page.NextCursor))
	if len(next.Assets) != 2 || next.Assets[0].Symbol != "G1009x" {
		t.Fatalf("second page = %+v", next)
	}
	narrow := pageOf(t, s.get(t, "/v1/assets?filter=all&sort=price&q=G1003"))
	if len(narrow.Assets) != 1 || narrow.Assets[0].Symbol != "G1003x" {
		t.Fatalf("q=G1003 = %+v", narrow)
	}
}

func TestListOrder_aWarmPriceCacheReadsOnlyTheTick(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedPriced(t)
	counter := &sampleCounter{}
	cfg := s.pool.Config().Copy()
	cfg.ConnConfig.Tracer = counter
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	l := app.NewListAssets(pool, s.clock)
	first, err := l.Handle(t.Context(), app.ListRequest{Filter: app.FilterAll, Sort: app.SortPrice, Limit: 4})
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	sparklines, newest := counter.sparkline.Load(), counter.newest.Load()
	if sparklines != 1 {
		t.Fatalf("cold load made %d sparkline queries, want 1", sparklines)
	}
	second, err := l.Handle(t.Context(), app.ListRequest{
		Filter: app.FilterAll, Sort: app.SortPrice, Limit: 4, Cursor: first.NextCursor,
	})
	if err != nil || len(second.Items) != 4 {
		t.Fatalf("second page = %+v, %v", second, err)
	}
	if counter.sparkline.Load() != sparklines || counter.newest.Load()-newest != 1 {
		t.Fatalf("warm page made %d sparkline and %d newest queries, want 0 and 1",
			counter.sparkline.Load()-sparklines, counter.newest.Load()-newest)
	}
}

func TestListOrder_noSortKeepsTheSymbolOrderAndOtherFiltersRefuseOne(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	plain := pageOf(t, s.get(t, "/v1/assets?filter=all"))
	explicit := pageOf(t, s.get(t, "/v1/assets?filter=all&sort=symbol"))
	symbols := make([]string, len(plain.Assets))
	for i, a := range plain.Assets {
		symbols[i] = a.Symbol
	}
	if !slices.IsSorted(symbols) || len(plain.Assets) != len(explicit.Assets) {
		t.Fatalf("default order %v is not symbol order", symbols)
	}
	for i := range plain.Assets {
		if plain.Assets[i].Symbol != explicit.Assets[i].Symbol {
			t.Fatalf("sort=symbol differs from no sort at %d", i)
		}
	}
	for _, path := range []string{
		"/v1/assets?filter=popular&sort=name",
		"/v1/assets?filter=movers&sort=price",
		"/v1/assets?filter=pre_ipo&sort=symbol",
		"/v1/assets?sort=newest",
	} {
		rec := s.get(t, path)
		if rec.Code != http.StatusBadRequest || problemCode(t, rec) != apibase.InvalidInput {
			t.Fatalf("%s = %d %s, want invalid_input", path, rec.Code, rec.Body)
		}
	}
}
