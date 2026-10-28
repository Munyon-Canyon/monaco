package app

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type heldClock struct{ clock.Real }

func (heldClock) Now() time.Time { return time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC) }

type heldCabalsFake struct {
	mine   []ids.CabalID
	status map[ids.CabalID]cabalport.Status
	failOn string
}

func (f heldCabalsFake) fail(op string) error {
	if f.failOn == op {
		return errs.New(errs.CodeDBUnavailable, "test."+op)
	}
	return nil
}

func (f heldCabalsFake) CabalsOf(context.Context, ids.UserID) ([]ids.CabalID, error) {
	return f.mine, f.fail("CabalsOf")
}

func (f heldCabalsFake) Cabals(_ context.Context, asked []ids.CabalID) (map[ids.CabalID]cabalport.CabalView, error) {
	out := map[ids.CabalID]cabalport.CabalView{}
	for _, id := range asked {
		if status, ok := f.status[id]; ok {
			out[id] = cabalport.CabalView{ID: id, Name: "cabal", Status: status}
		}
	}
	return out, f.fail("Cabals")
}

type heldPotsFake struct {
	pots []port.CabalPot
	err  error
}

func (f heldPotsFake) CabalPots(context.Context, []ids.CabalID) ([]port.CabalPot, error) {
	return f.pots, f.err
}

type heldLogosFake struct{ err error }

func (f heldLogosFake) LogosBySymbol(context.Context) (map[string]string, error) {
	return map[string]string{"A": "logo-a"}, f.err
}

func heldCabalID(n byte) ids.CabalID { return ids.CabalIDFrom(uuid.UUID{15: n}) }

func heldPot(prices time.Time, holdings ...port.Holding) port.CabalPot {
	return port.CabalPot{PricesAsOf: prices, Holdings: holdings}
}

func heldHolding(symbol string, value uint64) port.Holding {
	return port.Holding{Symbol: symbol, ValueMicros: money.MicrosFromUint64(value)}
}

func activeCabals(n ...byte) heldCabalsFake {
	f := heldCabalsFake{status: map[ids.CabalID]cabalport.Status{}}
	for _, b := range n {
		f.mine = append(f.mine, heldCabalID(b))
		f.status[heldCabalID(b)] = cabalport.StatusActive
	}
	return f
}

func TestHeldReads_refusesEveryFailedRead(t *testing.T) {
	t.Parallel()
	asOf := heldClock{}.Now()
	two := heldPotsFake{pots: []port.CabalPot{
		heldPot(asOf, heldHolding("A", math.MaxUint64)), heldPot(asOf, heldHolding("A", 1)),
	}}
	cases := map[string]*HeldReads{
		"cabals of": NewHeldReads(withFail(activeCabals(1), "CabalsOf"), two, heldLogosFake{}, heldClock{}),
		"cabals":    NewHeldReads(withFail(activeCabals(1), "Cabals"), two, heldLogosFake{}, heldClock{}),
		"pots": NewHeldReads(
			activeCabals(
				1,
				2,
			),
			heldPotsFake{err: errs.New(errs.CodeDBUnavailable, "test")},
			heldLogosFake{},
			heldClock{},
		),
		"logos": NewHeldReads(
			activeCabals(1, 2), two, heldLogosFake{err: errs.New(errs.CodeDBUnavailable, "test")}, heldClock{}),
		"total past uint64": NewHeldReads(activeCabals(1, 2), two, heldLogosFake{}, heldClock{}),
	}
	for name, reads := range cases {
		if _, err := reads.HeldBy(t.Context(), ids.UserID{}); err == nil {
			t.Fatalf("%s: HeldBy = nil, want an error", name)
		}
	}
}

func withFail(f heldCabalsFake, op string) heldCabalsFake {
	f.failOn = op
	return f
}

func TestHeldReads_ordersTiesBySymbolThenCabal(t *testing.T) {
	t.Parallel()
	asOf := heldClock{}.Now()
	pots := heldPotsFake{pots: []port.CabalPot{
		heldPot(asOf, heldHolding("B", 10), heldHolding("A", 10)),
		heldPot(asOf.Add(-time.Minute), heldHolding("A", 10)),
	}}
	got, err := NewHeldReads(activeCabals(2, 1), pots, heldLogosFake{}, heldClock{}).HeldBy(t.Context(), ids.UserID{})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		symbol string
		cabal  ids.CabalID
	}{{"A", heldCabalID(1)}, {"A", heldCabalID(2)}, {"B", heldCabalID(2)}}
	if len(got.Holdings) != len(want) {
		t.Fatalf("rows = %d, want %d", len(got.Holdings), len(want))
	}
	for i, w := range want {
		if row := got.Holdings[i]; row.Holding.Symbol != w.symbol || row.Cabal.ID != w.cabal {
			t.Fatalf("row %d = %s %s, want %s %s", i, row.Holding.Symbol, row.Cabal.ID, w.symbol, w.cabal)
		}
	}
	if got.Holdings[0].LogoURL != "logo-a" || got.Holdings[2].LogoURL != "" {
		t.Fatalf("logos = %q, %q", got.Holdings[0].LogoURL, got.Holdings[2].LogoURL)
	}
	if got.CabalCount != 2 || got.TotalValueMicros.Uint64() != 30 || !got.PricesAsOf.Equal(asOf.Add(-time.Minute)) {
		t.Fatalf("count %d total %d as of %v", got.CabalCount, got.TotalValueMicros.Uint64(), got.PricesAsOf)
	}
}

func TestHeldReads_aCallerWithoutActiveCabalsGetsAnEmptyList(t *testing.T) {
	t.Parallel()
	gone := activeCabals(1)
	gone.status[heldCabalID(1)] = cabalport.StatusBanned
	for name, cabals := range map[string]heldCabalsFake{"none": {}, "banned": gone, "unknown": {mine: gone.mine}} {
		got, err := NewHeldReads(cabals, heldPotsFake{}, heldLogosFake{}, heldClock{}).HeldBy(t.Context(), ids.UserID{})
		if err != nil || got.Holdings == nil || len(got.Holdings) != 0 || got.CabalCount != 0 ||
			!got.PricesAsOf.Equal(heldClock{}.Now()) {
			t.Fatalf("%s: HeldBy = %+v, %v", name, got, err)
		}
	}
}
