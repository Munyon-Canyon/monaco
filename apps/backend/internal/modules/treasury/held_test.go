package treasury_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/treasuryapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type heldCabals struct {
	mine  []ids.CabalID
	views map[ids.CabalID]cabalport.CabalView
	err   error
}

func (h heldCabals) CabalsOf(context.Context, ids.UserID) ([]ids.CabalID, error) {
	return h.mine, h.err
}

func (h heldCabals) Cabals(_ context.Context, asked []ids.CabalID) (map[ids.CabalID]cabalport.CabalView, error) {
	out := map[ids.CabalID]cabalport.CabalView{}
	for _, id := range asked {
		if view, ok := h.views[id]; ok {
			out[id] = view
		}
	}
	return out, nil
}

type heldLogos map[string]string

func (l heldLogos) LogosBySymbol(context.Context) (map[string]string, error) { return l, nil }

type heldCase struct {
	f      fixture
	viewer ids.UserID
	prices *marketfake.PricesFake
	reads  *int
	q      *adapters.Queries
	cabals heldCabals
}

func newHeldCase(t *testing.T) heldCase {
	t.Helper()
	f := newFixture(t)
	prices := &marketfake.PricesFake{}
	reads := 0
	reader := priceReader(prices)
	var mu sync.Mutex
	q := adapterQueriesWithReader(
		f,
		func(ctx context.Context, assetIDs []uuid.UUID, at time.Time) (map[uuid.UUID]app.Price, error) {
			mu.Lock()
			reads++
			mu.Unlock()
			got, err := reader(ctx, assetIDs, at)
			prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(999_000_000), at)
			return got, err
		},
	)
	now := f.clock.Now()
	prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(30_000_000), now)
	prices.Set(marketfake.TSLAx().ID, money.MicrosFromUint64(100_000_000), now.Add(-time.Minute))
	return heldCase{
		f: f, viewer: f.user(t), prices: prices, reads: &reads, q: q,
		cabals: heldCabals{views: map[ids.CabalID]cabalport.CabalView{}},
	}
}

func (h *heldCase) cabal(t *testing.T, name string, status cabalport.Status) ids.CabalID {
	t.Helper()
	id := h.f.cabal(t)
	h.cabals.views[id] = cabalport.CabalView{ID: id, Name: name, Status: status}
	h.cabals.mine = append(h.cabals.mine, id)
	return id
}

func (h heldCase) hold(t *testing.T, cabal ids.CabalID, asset market.Asset, units, cost int64) {
	t.Helper()
	const insert = `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
VALUES ($1, $2, $3, $4, $5)`
	_, err := h.f.pool.Exec(t.Context(), insert, cabal.UUID(), asset.Mint.String(), units, cost, h.f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
}

func (h heldCase) held() *app.HeldReads {
	return app.NewHeldReads(h.cabals, h.q, heldLogos{"AAPLx": "https://cdn.example.com/AAPLx.png"}, h.f.clock)
}

func (h heldCase) get(t *testing.T, symbol string) api.HeldList {
	t.Helper()
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: h.viewer.String()})
	req := api.GetMyHeldRequestObject{}
	if symbol != "" {
		req.Params.Symbol = &symbol
	}
	res, err := adapters.HTTP{Held: h.held()}.GetMyHeld(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return api.HeldList(res.(api.GetMyHeld200JSONResponse))
}

func TestGetMyHeld_ListsLargestFirstWithTotal(t *testing.T) {
	t.Parallel()
	h := newHeldCase(t)
	ac := h.cabal(t, "AC Capital", cabalport.StatusActive)
	degens := h.cabal(t, "Degens", cabalport.StatusActive)
	h.hold(t, ac, marketfake.AAPLx(), 200_000_000, 40_000_000)
	h.hold(t, ac, marketfake.TSLAx(), 150_000_000, 120_000_000)
	h.hold(t, degens, marketfake.AAPLx(), 100_000_000, 25_000_000)

	got := h.get(t, "")

	type row struct {
		cabal, symbol string
		value         int64
	}
	want := []row{
		{"AC Capital", "TSLAx", 150_000_000}, {"AC Capital", "AAPLx", 60_000_000}, {"Degens", "AAPLx", 30_000_000},
	}
	if len(got.Holdings) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(got.Holdings), len(want), got.Holdings)
	}
	var sum int64
	for i, w := range want {
		g := got.Holdings[i]
		if g.Cabal.Name != w.cabal || g.Holding.Symbol != w.symbol || g.Holding.ValueMicros != w.value {
			t.Fatalf("row %d = %s %s %d, want %+v", i, g.Cabal.Name, g.Holding.Symbol, g.Holding.ValueMicros, w)
		}
		sum += g.Holding.ValueMicros
	}
	if got.TotalValueMicros != sum || got.CabalCount != 2 {
		t.Fatalf("total %d cabals %d, want %d and 2", got.TotalValueMicros, got.CabalCount, sum)
	}
	if want := h.f.clock.Now().Add(-time.Minute); !got.PricesAsOf.Equal(want) {
		t.Fatalf("prices_as_of = %v, want the oldest price %v", got.PricesAsOf, want)
	}
	if got.Holdings[1].LogoUrl == nil || *got.Holdings[1].LogoUrl != "https://cdn.example.com/AAPLx.png" {
		t.Fatalf("AAPLx logo = %v", got.Holdings[1].LogoUrl)
	}
	if got.Holdings[0].LogoUrl != nil {
		t.Fatalf("TSLAx logo = %v, want none", *got.Holdings[0].LogoUrl)
	}
}

func TestGetMyHeld_SymbolFilterKeepsPotWeights(t *testing.T) {
	t.Parallel()
	h := newHeldCase(t)
	ac := h.cabal(t, "AC Capital", cabalport.StatusActive)
	h.hold(t, ac, marketfake.AAPLx(), 200_000_000, 40_000_000)
	h.hold(t, ac, marketfake.TSLAx(), 150_000_000, 120_000_000)

	got := h.get(t, "AAPLx")

	if len(got.Holdings) != 1 || got.Holdings[0].Holding.Symbol != "AAPLx" {
		t.Fatalf("rows = %+v, want only AAPLx", got.Holdings)
	}
	row := got.Holdings[0].Holding
	if row.WeightBps != 2857 || row.CostBasisMicros != 40_000_000 || got.TotalValueMicros != 60_000_000 {
		t.Fatalf("weight %d cost %d total %d, want 2857, 40000000, 60000000",
			row.WeightBps, row.CostBasisMicros, got.TotalValueMicros)
	}
}

func TestGetMyHeld_OnePriceReadForEveryCabal(t *testing.T) {
	t.Parallel()
	h := newHeldCase(t)
	for _, name := range []string{"One", "Two", "Three"} {
		h.hold(t, h.cabal(t, name, cabalport.StatusActive), marketfake.AAPLx(), 100_000_000, 25_000_000)
	}

	got := h.get(t, "")

	if *h.reads != 1 {
		t.Fatalf("price reads = %d, want 1", *h.reads)
	}
	if len(got.Holdings) != 3 {
		t.Fatalf("rows = %d, want 3", len(got.Holdings))
	}
	for _, row := range got.Holdings {
		if row.Holding.PriceMicros != 30_000_000 || row.Holding.ValueMicros != 30_000_000 {
			t.Fatalf("%s priced %d valued %d, want every row at the first read",
				row.Cabal.Name, row.Holding.PriceMicros, row.Holding.ValueMicros)
		}
	}
}

func TestGetMyHeld_EmptyWhenInNoActiveCabal(t *testing.T) {
	t.Parallel()
	h := newHeldCase(t)
	banned := h.cabal(t, "Gone", cabalport.StatusBanned)
	h.hold(t, banned, marketfake.AAPLx(), 100_000_000, 25_000_000)
	left := h.f.cabal(t)
	h.hold(t, left, marketfake.TSLAx(), 100_000_000, 25_000_000)

	got := h.get(t, "")

	if len(got.Holdings) != 0 || got.TotalValueMicros != 0 || got.CabalCount != 0 || got.Holdings == nil {
		t.Fatalf("got %+v, want an empty list", got)
	}
	if *h.reads != 0 {
		t.Fatalf("price reads = %d, want none", *h.reads)
	}
}

func TestGetMyHeld_SkipsSoldOutPositions(t *testing.T) {
	t.Parallel()
	h := newHeldCase(t)
	ac := h.cabal(t, "AC Capital", cabalport.StatusActive)
	h.hold(t, ac, marketfake.AAPLx(), 100_000_000, 25_000_000)
	h.hold(t, ac, marketfake.TSLAx(), 0, 0)

	got := h.get(t, "")

	if len(got.Holdings) != 1 || got.Holdings[0].Holding.Symbol != "AAPLx" {
		t.Fatalf("rows = %+v, want only AAPLx", got.Holdings)
	}
}

func TestCabalPots_MatchesCabalPot(t *testing.T) {
	t.Parallel()
	h := newHeldCase(t)
	ac := h.cabal(t, "AC Capital", cabalport.StatusActive)
	degens := h.cabal(t, "Degens", cabalport.StatusActive)
	h.hold(t, ac, marketfake.AAPLx(), 200_000_000, 40_000_000)
	h.hold(t, ac, marketfake.TSLAx(), 150_000_000, 120_000_000)
	h.hold(t, degens, marketfake.AAPLx(), 100_000_000, 25_000_000)
	h.prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(30_000_000), h.f.clock.Now())

	pots, err := h.q.CabalPots(t.Context(), []ids.CabalID{ac, degens})
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range []ids.CabalID{ac, degens} {
		h.prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(30_000_000), h.f.clock.Now())
		alone, err := h.q.CabalPot(t.Context(), id, h.viewer, false)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(pots[i], alone) {
			t.Fatalf("cabal %d pot differs:\n got %+v\nwant %+v", i, pots[i], alone)
		}
	}
}

var _ port.Held = (*app.HeldReads)(nil)

func TestGetMyHeld_refusesAnUnsignedCallerAndAFailedRead(t *testing.T) {
	t.Parallel()
	h := newHeldCase(t)
	routes := adapters.HTTP{Held: h.held()}
	if _, err := routes.GetMyHeld(
		t.Context(),
		api.GetMyHeldRequestObject{},
	); errs.CodeOf(
		err,
	) != errs.CodeUnauthorized {
		t.Fatalf("GetMyHeld without an actor = %v, want unauthorized", err)
	}
	h.cabals.err = errs.New(errs.CodeDBUnavailable, "test")
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: h.viewer.String()})
	if _, err := (adapters.HTTP{Held: h.held()}).GetMyHeld(ctx, api.GetMyHeldRequestObject{}); err == nil {
		t.Fatal("GetMyHeld over a failed read = nil, want an error")
	}
}
