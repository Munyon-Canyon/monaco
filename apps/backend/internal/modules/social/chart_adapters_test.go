package social

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	marketport "github.com/monaco/monaco/apps/backend/internal/modules/market/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type historyStub struct {
	latest  map[uuid.UUID]money.Micros
	closes  map[uuid.UUID][]marketport.Bucketed
	err     error
	windows []marketport.Window
	buckets int
}

func (h *historyStub) Latest(context.Context, []uuid.UUID) (map[uuid.UUID]money.Micros, error) {
	return h.latest, h.err
}

func (h *historyStub) Closes(
	_ context.Context, windows []marketport.Window, buckets int,
) (map[uuid.UUID][]marketport.Bucketed, error) {
	h.windows, h.buckets = windows, buckets
	return h.closes, h.err
}

type marksStub struct {
	marks map[treasuryport.CabalAsset][]treasuryport.TradeMark
	err   error
	pairs []treasuryport.CabalAsset
}

func (m *marksStub) TradeMarks(
	_ context.Context, pairs []treasuryport.CabalAsset,
) (map[treasuryport.CabalAsset][]treasuryport.TradeMark, error) {
	m.pairs = pairs
	return m.marks, m.err
}

func TestChartPrices_passesLatestThroughAndConvertsCloses(t *testing.T) {
	t.Parallel()
	gen := testkit.NewIDs(6527)
	asset := gen.NewV7()
	key := gen.NewV7()
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	stub := &historyStub{
		latest: map[uuid.UUID]money.Micros{asset: money.MicrosFromUint64(7)},
		closes: map[uuid.UUID][]marketport.Bucketed{key: {{At: from, PriceMicros: money.MicrosFromUint64(3)}}},
	}
	prices := chartPrices{stub}
	latest, err := prices.Latest(t.Context(), []uuid.UUID{asset})
	if err != nil || !reflect.DeepEqual(latest, stub.latest) {
		t.Fatalf("latest = %v, %v", latest, err)
	}
	got, err := prices.Closes(t.Context(), []app.ChartWindow{{Key: key, AssetID: asset, From: from, To: to}}, 4)
	want := map[uuid.UUID][]app.ChartPoint{key: {{At: from, PriceMicros: money.MicrosFromUint64(3)}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("closes = %v, %v, want %v", got, err, want)
	}
	wantWindows := []marketport.Window{{Key: key, AssetID: asset, From: from, To: to}}
	if !reflect.DeepEqual(stub.windows, wantWindows) || stub.buckets != 4 {
		t.Fatalf("windows = %+v buckets = %d", stub.windows, stub.buckets)
	}
}

func TestChartPrices_closesReturnsTheHistoryError(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test.boom")
	got, err := chartPrices{&historyStub{err: boom}}.Closes(t.Context(), nil, 1)
	if !errors.Is(err, boom) || got != nil {
		t.Fatalf("closes = %v, %v, want boom", got, err)
	}
}

func TestChartTrades_convertsMarksPerPair(t *testing.T) {
	t.Parallel()
	gen := testkit.NewIDs(6528)
	cabal := ids.CabalIDFrom(gen.NewV7())
	at := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	cycle := at.Add(-time.Hour)
	stub := &marksStub{marks: map[treasuryport.CabalAsset][]treasuryport.TradeMark{
		{CabalID: cabal, Symbol: "META"}: {{
			At: at, Side: "sell", USDCMicros: money.MicrosFromUint64(5), PriceMicros: money.MicrosFromUint64(2),
			CostOfSoldMicros: money.MicrosFromUint64(4), CycleStart: cycle,
		}},
	}}
	got, err := chartTrades{stub}.TradeMarks(t.Context(), []app.ChartPair{{CabalID: cabal, Symbol: "META"}})
	want := map[app.ChartPair][]app.ChartTrade{{CabalID: cabal, Symbol: "META"}: {{
		At: at, Side: "sell", USDCMicros: money.MicrosFromUint64(5), PriceMicros: money.MicrosFromUint64(2),
		CostOfSoldMicros: money.MicrosFromUint64(4), CycleStart: cycle,
	}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("marks = %v, %v, want %v", got, err, want)
	}
	if !reflect.DeepEqual(stub.pairs, []treasuryport.CabalAsset{{CabalID: cabal, Symbol: "META"}}) {
		t.Fatalf("pairs = %+v", stub.pairs)
	}
}

func TestChartTrades_returnsTheMarksError(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test.boom")
	got, err := chartTrades{&marksStub{err: boom}}.TradeMarks(t.Context(), nil)
	if !errors.Is(err, boom) || got != nil {
		t.Fatalf("marks = %v, %v, want boom", got, err)
	}
}
