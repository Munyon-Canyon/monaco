package adapters

import (
	"context"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	chartBuckets   = 32
	chartMarkers   = 6
	saleMatchSlack = time.Minute
	bpsScale       = 10_000
	sideBuy        = "buy"
	sideSell       = "sell"
)

type FeedChartView struct {
	ReturnBps *int64
	Chart     *api.FeedChart
}

type chartSubject struct {
	item app.FeedItem
	pair app.ChartPair
}

type chartPlan struct {
	id      uuid.UUID
	bps     int64
	markers []api.FeedChartMarker
}

func (h HTTP) loadCharts(
	ctx context.Context, items []app.FeedItem, now time.Time,
) (map[uuid.UUID]FeedChartView, error) {
	return LoadFeedCharts(ctx, h.History, h.Marks, items, now)
}

func LoadFeedCharts(
	ctx context.Context, prices app.ChartPrices, trades app.ChartTrades, items []app.FeedItem, now time.Time,
) (map[uuid.UUID]FeedChartView, error) {
	const op = "social.loadCharts"
	out := map[uuid.UUID]FeedChartView{}
	subjects := chartSubjects(items)
	if len(subjects) == 0 {
		return out, nil
	}
	marks, err := loadChartMarks(ctx, trades, subjects)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	var assetIDs []uuid.UUID
	for _, s := range subjects {
		if !slices.Contains(assetIDs, s.item.AssetID) {
			assetIDs = append(assetIDs, s.item.AssetID)
		}
	}
	latest, err := prices.Latest(ctx, assetIDs)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	var plans []chartPlan
	var windows []app.ChartWindow
	for _, s := range subjects {
		plan, start, ok := planChart(s, latest[s.item.AssetID], marks[s.pair], now)
		if !ok {
			continue
		}
		plans = append(plans, plan)
		windows = append(windows, app.ChartWindow{Key: s.item.ID, AssetID: s.item.AssetID, From: start, To: now})
	}
	if len(windows) == 0 {
		return out, nil
	}
	series, err := prices.Closes(ctx, windows, chartBuckets)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	for _, p := range plans {
		bps := p.bps
		view := FeedChartView{ReturnBps: &bps}
		if points := series[p.id]; len(points) > 0 {
			view.Chart = &api.FeedChart{Points: wireChartPoints(points), Markers: p.markers}
		}
		out[p.id] = view
	}
	return out, nil
}

func chartSubjects(items []app.FeedItem) []chartSubject {
	var out []chartSubject
	for _, item := range items {
		eligible := item.Kind == feed.KindResult && item.Outcome == feed.OutcomePassed &&
			item.FillState == feed.FillFilled && item.AssetKind == feed.AssetKindEquity &&
			item.FilledAt != nil && item.AssetID != uuid.Nil &&
			(item.Action == feed.ActionBuy || item.Action == feed.ActionSell)
		if eligible {
			out = append(out, chartSubject{item: item, pair: app.ChartPair{CabalID: item.CabalID, Symbol: item.Symbol}})
		}
	}
	return out
}

func loadChartMarks(
	ctx context.Context, trades app.ChartTrades, subjects []chartSubject,
) (map[app.ChartPair][]app.ChartTrade, error) {
	var pairs []app.ChartPair
	for _, s := range subjects {
		if s.item.Action == feed.ActionSell && !slices.Contains(pairs, s.pair) {
			pairs = append(pairs, s.pair)
		}
	}
	if len(pairs) == 0 {
		return map[app.ChartPair][]app.ChartTrade{}, nil
	}
	return trades.TradeMarks(ctx, pairs)
}

func planChart(
	s chartSubject,
	latest money.Micros,
	trades []app.ChartTrade,
	now time.Time,
) (chartPlan, time.Time, bool) {
	if latest.IsZero() {
		return chartPlan{}, time.Time{}, false
	}
	if s.item.Action == feed.ActionBuy {
		bps, ok := signedBps(latest, s.item.PriceMicros)
		return chartPlan{id: s.item.ID, bps: bps}, *s.item.FilledAt, ok
	}
	at, found := saleIndex(trades, *s.item.FilledAt)
	if !found {
		return chartPlan{}, time.Time{}, false
	}
	sale := trades[at]
	bps, ok := signedBps(sale.USDCMicros, sale.CostOfSoldMicros)
	if !ok {
		return chartPlan{}, time.Time{}, false
	}
	return chartPlan{
		id:      s.item.ID,
		bps:     bps,
		markers: pickMarkers(trades, at, sale.CycleStart, now),
	}, sale.CycleStart, true
}

func saleIndex(trades []app.ChartTrade, filledAt time.Time) (int, bool) {
	best, gap := -1, saleMatchSlack+1
	for i, t := range trades {
		if t.Side != sideSell {
			continue
		}
		if d := t.At.Sub(filledAt).Abs(); d < gap {
			best, gap = i, d
		}
	}
	return best, best >= 0
}

func signedBps(value, base money.Micros) (int64, bool) {
	if base.IsZero() {
		return 0, false
	}
	magnitude, negative := value.Uint64()-base.Uint64(), false
	if value.Cmp(base) < 0 {
		magnitude, negative = base.Uint64()-value.Uint64(), true
	}
	bps, err := money.MulDiv(magnitude, bpsScale, base.Uint64())
	if err != nil || bps > math.MaxInt64 {
		return 0, false
	}
	if negative {
		return -int64(bps), true
	}
	return int64(bps), true
}

func pickMarkers(trades []app.ChartTrade, saleAt int, start, now time.Time) []api.FeedChartMarker {
	var inside []int
	firstBuy := -1
	for i, t := range trades {
		if t.At.Before(start) || t.At.After(now) {
			continue
		}
		if firstBuy < 0 && t.Side == sideBuy {
			firstBuy = i
		}
		inside = append(inside, i)
	}
	keep := map[int]bool{saleAt: true}
	if firstBuy >= 0 {
		keep[firstBuy] = true
	}
	for _, i := range slices.Backward(inside) {
		if len(keep) >= chartMarkers {
			break
		}
		keep[i] = true
	}
	out := make([]api.FeedChartMarker, 0, len(keep))
	for _, i := range inside {
		if keep[i] {
			t := trades[i]
			out = append(out, api.FeedChartMarker{
				At: t.At, Side: api.FeedItemChartMarkersSide(t.Side), PriceMicros: chartMicros(t.PriceMicros),
			})
		}
	}
	return out
}

func wireChartPoints(points []app.ChartPoint) []api.FeedChartPoint {
	out := make([]api.FeedChartPoint, len(points))
	for i, p := range points {
		out[i] = api.FeedChartPoint{At: p.At, PriceMicros: chartMicros(p.PriceMicros)}
	}
	return out
}

func chartMicros(m money.Micros) int64 {
	return int64(min(m.Uint64(), math.MaxInt64))
}
