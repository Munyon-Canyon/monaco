package social_test

import (
	"context"
	"crypto/rand"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type chartPricesFake struct {
	latest       map[uuid.UUID]money.Micros
	latestCalls  int
	closesCalls  int
	windows      []app.ChartWindow
	empty        bool
	latestErr    error
	closesErr    error
	requestedFor []uuid.UUID
}

func (f *chartPricesFake) Latest(_ context.Context, assetIDs []uuid.UUID) (map[uuid.UUID]money.Micros, error) {
	f.latestCalls++
	f.requestedFor = assetIDs
	return f.latest, f.latestErr
}

func (f *chartPricesFake) Closes(
	_ context.Context, windows []app.ChartWindow, buckets int,
) (map[uuid.UUID][]app.ChartPoint, error) {
	f.closesCalls++
	f.windows = windows
	if f.closesErr != nil {
		return nil, f.closesErr
	}
	out := map[uuid.UUID][]app.ChartPoint{}
	for _, w := range windows {
		if f.empty {
			continue
		}
		out[w.Key] = []app.ChartPoint{
			{At: w.From, PriceMicros: money.MicrosFromUint64(uint64(max(buckets, 0)))},
			{At: w.To, PriceMicros: money.MicrosFromUint64(uint64(max(buckets, 0)) + 1)},
		}
	}
	return out, nil
}

type chartTradesFake struct {
	marks map[app.ChartPair][]app.ChartTrade
	calls int
	pairs []app.ChartPair
	err   error
}

func (f *chartTradesFake) TradeMarks(
	_ context.Context, pairs []app.ChartPair,
) (map[app.ChartPair][]app.ChartTrade, error) {
	f.calls++
	f.pairs = pairs
	return f.marks, f.err
}

func micros(v uint64) money.Micros { return money.MicrosFromUint64(v) }

func chartItem(action feed.Action, asset uuid.UUID, cabal ids.CabalID, filledAt time.Time, price uint64) app.FeedItem {
	return app.FeedItem{
		ID: nextID(), Kind: feed.KindResult, Outcome: feed.OutcomePassed, FillState: feed.FillFilled,
		AssetKind: feed.AssetKindEquity, FilledAt: &filledAt, AssetID: asset, CabalID: cabal, Symbol: "META",
		Action: action, PriceMicros: micros(price),
	}
}

func trade(side string, at time.Time, price, usdc, cost uint64, cycle time.Time) app.ChartTrade {
	return app.ChartTrade{
		At: at, Side: side, USDCMicros: micros(usdc), PriceMicros: micros(price), CostOfSoldMicros: micros(cost),
		CycleStart: cycle,
	}
}

func chartNow() time.Time { return time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC) }

func nextID() uuid.UUID {
	var id uuid.UUID
	if _, err := rand.Read(id[:]); err != nil {
		panic(err)
	}
	return id
}

func chartLoad(
	t *testing.T, p *chartPricesFake, tr *chartTradesFake, items ...app.FeedItem,
) map[uuid.UUID]adapters.FeedChartView {
	t.Helper()
	out, err := adapters.LoadFeedCharts(t.Context(), p, tr, items, chartNow())
	if err != nil {
		t.Fatalf("LoadFeedCharts: %v", err)
	}
	return out
}

func wantBps(t *testing.T, got adapters.FeedChartView, want int64) {
	t.Helper()
	if got.ReturnBps == nil || *got.ReturnBps != want {
		t.Fatalf("return_bps = %v, want %d", got.ReturnBps, want)
	}
}

func wantNull(t *testing.T, got adapters.FeedChartView, ok bool) {
	t.Helper()
	if ok || got.ReturnBps != nil || got.Chart != nil {
		t.Fatalf("view = %+v (present %v), want null", got, ok)
	}
}

func TestFeedChart_buyReturnsSinceTheFillWithNoMarkers(t *testing.T) {
	t.Parallel()
	asset, cabal := nextID(), ids.CabalIDFrom(nextID())
	fill := chartNow().Add(-48 * time.Hour)
	item := chartItem(feed.ActionBuy, asset, cabal, fill, 716_520_000)
	p := &chartPricesFake{latest: map[uuid.UUID]money.Micros{asset: micros(724_400_000)}}
	tr := &chartTradesFake{}
	got := chartLoad(t, p, tr, item)[item.ID]
	wantBps(t, got, 109)
	if got.Chart == nil || len(got.Chart.Points) != 2 || len(got.Chart.Markers) != 0 {
		t.Fatalf("chart = %+v", got.Chart)
	}
	if got.Chart.Points[0].At.Before(fill) {
		t.Fatalf("chart starts before the fill: %v", got.Chart.Points[0].At)
	}
	w := p.windows[0]
	if w.Key != item.ID || w.AssetID != asset || !w.From.Equal(fill) || !w.To.Equal(chartNow()) {
		t.Fatalf("window = %+v", w)
	}
	if tr.calls != 0 {
		t.Fatalf("TradeMarks called %d times for a buy-only page", tr.calls)
	}
}

func TestFeedChart_lossTruncatesTowardZero(t *testing.T) {
	t.Parallel()
	asset := nextID()
	item := chartItem(feed.ActionBuy, asset, ids.CabalIDFrom(nextID()), chartNow().Add(-time.Hour), 724_400_000)
	p := &chartPricesFake{latest: map[uuid.UUID]money.Micros{asset: micros(716_520_000)}}
	wantBps(t, chartLoad(t, p, &chartTradesFake{}, item)[item.ID], -108)
}

func TestFeedChart_saleReturnsGainOverCostAndMarksTheCycle(t *testing.T) {
	t.Parallel()
	asset, cabal := nextID(), ids.CabalIDFrom(nextID())
	boughtAt, soldAt := chartNow().Add(-72*time.Hour), chartNow().Add(-24*time.Hour)
	item := chartItem(feed.ActionSell, asset, cabal, soldAt, 240_000_000)
	pair := app.ChartPair{CabalID: cabal, Symbol: "META"}
	tr := &chartTradesFake{marks: map[app.ChartPair][]app.ChartTrade{pair: {
		trade("buy", boughtAt, 224_925_000, 899_700_000, 0, boughtAt),
		trade("sell", soldAt, 240_000_000, 480_000_000, 449_850_000, boughtAt),
	}}}
	p := &chartPricesFake{latest: map[uuid.UUID]money.Micros{asset: micros(250_000_000)}}
	got := chartLoad(t, p, tr, item)[item.ID]
	wantBps(t, got, 670)
	if len(got.Chart.Markers) != 2 || string(got.Chart.Markers[0].Side) != "buy" ||
		string(got.Chart.Markers[1].Side) != "sell" {
		t.Fatalf("markers = %+v", got.Chart.Markers)
	}
	if got.Chart.Markers[1].PriceMicros != 240_000_000 {
		t.Fatalf("sell marker price = %d", got.Chart.Markers[1].PriceMicros)
	}
	if !p.windows[0].From.Equal(boughtAt) {
		t.Fatalf("sale window starts %v, want the cycle's first buy %v", p.windows[0].From, boughtAt)
	}
}

func TestFeedChart_saleAfterAnEmptiedPositionStartsAtTheFirstBuyAndKeepsItsSell(t *testing.T) {
	t.Parallel()
	asset, cabal := nextID(), ids.CabalIDFrom(nextID())
	b1, s1, b2 := chartNow().Add(-72*time.Hour), chartNow().Add(-48*time.Hour), chartNow().Add(-24*time.Hour)
	item := chartItem(feed.ActionSell, asset, cabal, s1, 1)
	pair := app.ChartPair{CabalID: cabal, Symbol: "META"}
	tr := &chartTradesFake{marks: map[app.ChartPair][]app.ChartTrade{pair: {
		trade("buy", b1, 100, 100, 0, b1),
		trade("sell", s1, 110, 110, 100, b1),
		trade("buy", b2, 105, 105, 0, b2),
	}}}
	p := &chartPricesFake{latest: map[uuid.UUID]money.Micros{asset: micros(120)}}
	got := chartLoad(t, p, tr, item)[item.ID]
	wantBps(t, got, 1000)
	if !p.windows[0].From.Equal(b1) {
		t.Fatalf("window from %v, want first buy %v", p.windows[0].From, b1)
	}
	sides := make([]string, 0, len(got.Chart.Markers))
	for _, m := range got.Chart.Markers {
		sides = append(sides, string(m.Side))
	}
	if len(sides) != 3 || sides[0] != "buy" || sides[1] != "sell" || sides[2] != "buy" {
		t.Fatalf("sides = %v", sides)
	}
}

func TestFeedChart_markersCapAtSixKeepingTheSaleAndItsFirstBuy(t *testing.T) {
	t.Parallel()
	asset, cabal := nextID(), ids.CabalIDFrom(nextID())
	start := chartNow().Add(-100 * time.Hour)
	var list []app.ChartTrade
	list = append(list, trade("buy", start, 1, 1, 0, start))
	for i := 1; i <= 9; i++ {
		list = append(list, trade("buy", start.Add(time.Duration(i)*time.Hour), uint64(10+i), 1, 0, start))
	}
	saleAt := start.Add(2 * time.Hour)
	list[2] = trade("sell", saleAt, 12, 2, 1, start)
	item := chartItem(feed.ActionSell, asset, cabal, saleAt, 1)
	pair := app.ChartPair{CabalID: cabal, Symbol: "META"}
	tr := &chartTradesFake{marks: map[app.ChartPair][]app.ChartTrade{pair: list}}
	p := &chartPricesFake{latest: map[uuid.UUID]money.Micros{asset: micros(3)}}
	got := chartLoad(t, p, tr, item)[item.ID]
	markers := got.Chart.Markers
	if len(markers) != 6 {
		t.Fatalf("markers = %d, want 6", len(markers))
	}
	if !markers[0].At.Equal(start) || !markers[1].At.Equal(saleAt) || string(markers[1].Side) != "sell" {
		t.Fatalf("first buy and sale not kept: %+v", markers[:2])
	}
	for i, m := range markers[2:] {
		want := start.Add(time.Duration(6+i) * time.Hour)
		if !m.At.Equal(want) {
			t.Fatalf("marker %d at %v, want newest others from %v", i+2, m.At, want)
		}
	}
}

func TestFeedChart_markersStayInsideTheWindow(t *testing.T) {
	t.Parallel()
	asset, cabal := nextID(), ids.CabalIDFrom(nextID())
	b, s := chartNow().Add(-72*time.Hour), chartNow().Add(-24*time.Hour)
	item := chartItem(feed.ActionSell, asset, cabal, s, 1)
	pair := app.ChartPair{CabalID: cabal, Symbol: "META"}
	tr := &chartTradesFake{marks: map[app.ChartPair][]app.ChartTrade{pair: {
		trade("buy", b.Add(-time.Hour), 1, 1, 0, b.Add(-time.Hour)),
		trade("buy", b, 1, 1, 0, b),
		trade("sell", s, 2, 2, 1, b),
		trade("buy", chartNow().Add(time.Hour), 1, 1, 0, chartNow().Add(time.Hour)),
	}}}
	p := &chartPricesFake{latest: map[uuid.UUID]money.Micros{asset: micros(3)}}
	if n := len(chartLoad(t, p, tr, item)[item.ID].Chart.Markers); n != 2 {
		t.Fatalf("markers = %d, want 2", n)
	}
}

func TestFeedChart_samePageSameAssetGetsOneChartPerItem(t *testing.T) {
	t.Parallel()
	asset, cabal := nextID(), ids.CabalIDFrom(nextID())
	a := chartItem(feed.ActionBuy, asset, cabal, chartNow().Add(-24*time.Hour), 100)
	b := chartItem(feed.ActionBuy, asset, cabal, chartNow().Add(-8*24*time.Hour), 100)
	p := &chartPricesFake{latest: map[uuid.UUID]money.Micros{asset: micros(110)}}
	got := chartLoad(t, p, &chartTradesFake{}, a, b)
	if got[a.ID].Chart.Points[0].At.Equal(got[b.ID].Chart.Points[0].At) {
		t.Fatal("two items on one asset share a chart start")
	}
	if !got[b.ID].Chart.Points[0].At.Equal(chartNow().Add(-8 * 24 * time.Hour)) {
		t.Fatalf("second chart starts %v", got[b.ID].Chart.Points[0].At)
	}
	if len(p.requestedFor) != 1 {
		t.Fatalf("Latest asked for %d assets, want 1", len(p.requestedFor))
	}
}

func TestFeedChart_aPageOfThirtyItemsMakesThreeCalls(t *testing.T) {
	t.Parallel()
	asset, cabal := nextID(), ids.CabalIDFrom(nextID())
	pair := app.ChartPair{CabalID: cabal, Symbol: "META"}
	sold := chartNow().Add(-time.Hour)
	var items []app.FeedItem
	for i := range 30 {
		switch {
		case i < 3:
			items = append(
				items,
				chartItem(feed.ActionBuy, asset, cabal, chartNow().Add(-time.Duration(i+2)*time.Hour), 100),
			)
		case i < 5:
			items = append(items, chartItem(feed.ActionSell, asset, cabal, sold, 100))
		default:
			items = append(items, app.FeedItem{ID: nextID(), Kind: feed.KindProposal})
		}
	}
	items = append(items, app.FeedItem{ID: nextID(), Kind: feed.KindResult, Outcome: feed.OutcomeFailed})
	tr := &chartTradesFake{marks: map[app.ChartPair][]app.ChartTrade{pair: {
		trade("buy", sold.Add(-time.Hour), 90, 90, 0, sold.Add(-time.Hour)),
		trade("sell", sold, 100, 100, 90, sold.Add(-time.Hour)),
	}}}
	p := &chartPricesFake{latest: map[uuid.UUID]money.Micros{asset: micros(110)}}
	got := chartLoad(t, p, tr, items...)
	if p.latestCalls != 1 || p.closesCalls != 1 || tr.calls != 1 {
		t.Fatalf("calls: latest %d closes %d marks %d, want 1 each", p.latestCalls, p.closesCalls, tr.calls)
	}
	if len(tr.pairs) != 1 || len(p.windows) != 5 || len(got) != 5 {
		t.Fatalf("pairs %d windows %d views %d", len(tr.pairs), len(p.windows), len(got))
	}
}

func TestFeedChart_unchartableResultsAreNullAndMakeNoCalls(t *testing.T) {
	t.Parallel()
	asset, cabal := nextID(), ids.CabalIDFrom(nextID())
	base := func() app.FeedItem { return chartItem(feed.ActionBuy, asset, cabal, chartNow().Add(-time.Hour), 100) }
	preIPO, failed, pending, partial, noFill, noAsset, other := base(), base(), base(), base(), base(), base(), base()
	preIPO.AssetKind = feed.AssetKindPreIPO
	failed.Outcome = feed.OutcomeFailed
	pending.FillState = feed.FillPending
	partial.FillState = feed.FillPartial
	noFill.FilledAt = nil
	noAsset.AssetID = uuid.Nil
	other.Kind = feed.KindProposal
	p, tr := &chartPricesFake{}, &chartTradesFake{}
	items := []app.FeedItem{preIPO, failed, pending, partial, noFill, noAsset, other}
	got := chartLoad(t, p, tr, items...)
	for _, item := range items {
		v, ok := got[item.ID]
		wantNull(t, v, ok)
	}
	if p.latestCalls+p.closesCalls+tr.calls != 0 {
		t.Fatal("a page with no chartable result read prices or trades")
	}
}

func TestFeedChart_anUnpricedAssetNullsOnlyItsOwnItem(t *testing.T) {
	t.Parallel()
	priced, unpriced, cabal := nextID(), nextID(), ids.CabalIDFrom(nextID())
	good := chartItem(feed.ActionBuy, priced, cabal, chartNow().Add(-time.Hour), 100)
	bad := chartItem(feed.ActionBuy, unpriced, cabal, chartNow().Add(-time.Hour), 100)
	p := &chartPricesFake{latest: map[uuid.UUID]money.Micros{priced: micros(110)}}
	got := chartLoad(t, p, &chartTradesFake{}, good, bad)
	wantBps(t, got[good.ID], 1000)
	v, ok := got[bad.ID]
	wantNull(t, v, ok)
}

func TestFeedChart_noPricedItemSkipsTheSeriesRead(t *testing.T) {
	t.Parallel()
	item := chartItem(feed.ActionBuy, nextID(), ids.CabalIDFrom(nextID()), chartNow().Add(-time.Hour), 100)
	p := &chartPricesFake{}
	v, ok := chartLoad(t, p, &chartTradesFake{}, item)[item.ID]
	wantNull(t, v, ok)
	if p.closesCalls != 0 {
		t.Fatal("Closes called with no window")
	}
}

func TestFeedChart_unknownFiguresAreNull(t *testing.T) {
	t.Parallel()
	asset, cabal := nextID(), ids.CabalIDFrom(nextID())
	pair := app.ChartPair{CabalID: cabal, Symbol: "META"}
	at := chartNow().Add(-time.Hour)
	zeroPrice := chartItem(feed.ActionBuy, asset, cabal, at, 0)
	hugeGain := chartItem(feed.ActionBuy, asset, cabal, at, 1)
	noMark := chartItem(feed.ActionSell, asset, cabal, chartNow().Add(-2*time.Hour), 1)
	zeroCost := chartItem(feed.ActionSell, asset, cabal, at, 1)
	tr := &chartTradesFake{marks: map[app.ChartPair][]app.ChartTrade{pair: {
		trade("buy", at.Add(-time.Hour), 1, 1, 0, at.Add(-time.Hour)),
		trade("sell", at, 1, 1, 0, at.Add(-time.Hour)),
	}}}
	p := &chartPricesFake{latest: map[uuid.UUID]money.Micros{asset: micros(1_000_000_000_000_000)}}
	got := chartLoad(t, p, tr, zeroPrice, hugeGain, noMark, zeroCost)
	for _, item := range []app.FeedItem{zeroPrice, hugeGain, noMark, zeroCost} {
		v, ok := got[item.ID]
		wantNull(t, v, ok)
	}
}

func TestFeedChart_overflowingFigureIsNull(t *testing.T) {
	t.Parallel()
	asset, cabal := nextID(), ids.CabalIDFrom(nextID())
	item := chartItem(feed.ActionBuy, asset, cabal, chartNow().Add(-time.Hour), 1)
	p := &chartPricesFake{latest: map[uuid.UUID]money.Micros{asset: micros(math.MaxUint64)}}
	v, ok := chartLoad(t, p, &chartTradesFake{}, item)[item.ID]
	wantNull(t, v, ok)
}

func TestFeedChart_noSeriesKeepsTheReturn(t *testing.T) {
	t.Parallel()
	asset := nextID()
	item := chartItem(feed.ActionBuy, asset, ids.CabalIDFrom(nextID()), chartNow().Add(-time.Hour), 100)
	p := &chartPricesFake{latest: map[uuid.UUID]money.Micros{asset: micros(110)}, empty: true}
	got := chartLoad(t, p, &chartTradesFake{}, item)[item.ID]
	wantBps(t, got, 1000)
	if got.Chart != nil {
		t.Fatalf("chart = %+v, want null", got.Chart)
	}
}

func TestFeedChart_readErrorsFailThePage(t *testing.T) {
	t.Parallel()
	asset, cabal := nextID(), ids.CabalIDFrom(nextID())
	pair := app.ChartPair{CabalID: cabal, Symbol: "META"}
	at := chartNow().Add(-time.Hour)
	sale := chartItem(feed.ActionSell, asset, cabal, at, 1)
	boom := errs.New(errs.CodeInternal, "test.boom")
	marks := map[app.ChartPair][]app.ChartTrade{pair: {trade("sell", at, 2, 2, 1, at)}}
	for name, tc := range map[string]struct {
		p  *chartPricesFake
		tr *chartTradesFake
	}{
		"trade marks": {&chartPricesFake{}, &chartTradesFake{err: boom}},
		"latest":      {&chartPricesFake{latestErr: boom}, &chartTradesFake{marks: marks}},
		"closes": {
			&chartPricesFake{latest: map[uuid.UUID]money.Micros{asset: micros(3)}, closesErr: boom},
			&chartTradesFake{marks: marks},
		},
	} {
		if _, err := adapters.LoadFeedCharts(
			t.Context(),
			tc.p,
			tc.tr,
			[]app.FeedItem{sale},
			chartNow(),
		); !errors.Is(
			err,
			boom,
		) {
			t.Errorf("%s: err = %v, want boom", name, err)
		}
	}
}

func TestGetFeed_aChartReadErrorFailsTheList(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer := ids.NewUserID(f.gen)
	asset := f.gen.NewV7()
	ref := f.gen.NewV7()
	f.item(t, func(it *feed.Item) {
		it.Kind, it.RefID, it.AssetID, it.CabalID = feed.KindResult, ref, asset, ids.CabalIDFrom(f.gen.NewV7())
		it.Payload = feed.Payload{
			Symbol: "META", Action: feed.ActionBuy, Outcome: feed.OutcomePassed, AssetKind: feed.AssetKindEquity,
			ExpectedFills: 1,
		}
		it.Payload.Fills = map[string]feed.Fill{
			f.gen.NewV7().String(): {
				TokenAmount: money.NewBaseUnits(1_000_000, 6), USDCMicros: micros(2_000_000), FilledAt: f.clock.Now(),
			},
		}
	})
	boom := errs.New(errs.CodeInternal, "test.boom")
	routes := f.routes()
	routes.History = &chartPricesFake{latestErr: boom}
	_, err := routes.GetFeed(asUser(t.Context(), viewer), api.GetFeedRequestObject{})
	if !errors.Is(err, boom) {
		t.Fatalf("GetFeed err = %v, want boom", err)
	}
}
