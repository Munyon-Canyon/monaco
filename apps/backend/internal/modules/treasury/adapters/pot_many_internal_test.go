package adapters

import (
	"context"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func twoCabals() []ids.CabalID {
	g := testkit.NewIDs(9)
	return []ids.CabalID{ids.CabalIDFrom(g.NewV7()), ids.CabalIDFrom(g.NewV7())}
}

func TestCabalPots_refusesBadReads(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx().Mint.String()
	down := errs.New(errs.CodeDBUnavailable, "test")
	cases := map[string]struct {
		store     potFake
		pricesErr error
		listErr   error
	}{
		"snapshot":     {store: potFake{snapshotErr: down}},
		"chain rows":   {store: potFake{snapshot: []sqlc.CabalPotSnapshotRow{potRow(evmStock, "x", "0")}}},
		"unknown mint": {store: potFake{snapshot: []sqlc.CabalPotSnapshotRow{potRow("bad", "1", "1")}}},
		"prices": {
			store:     potFake{snapshot: []sqlc.CabalPotSnapshotRow{potRow(aapl, "1", "1")}},
			pricesErr: down,
		},
		"listings": {
			store:   potFake{snapshot: []sqlc.CabalPotSnapshotRow{potRow(evmStock, "1", "1")}},
			listErr: down,
		},
		"reserved cash": {
			store: potFake{snapshot: []sqlc.CabalPotSnapshotRow{withReserved(potRow("", "0", "0"), "x")}},
		},
	}
	for name, c := range cases {
		q := potQueries(c.store, c.pricesErr)
		q.WithListings(func(context.Context, []string, time.Time) (map[string]app.ListingQuote, error) {
			return nil, c.listErr
		})
		if _, err := q.CabalPots(t.Context(), twoCabals()); err == nil {
			t.Fatalf("%s: CabalPots = nil, want an error", name)
		}
	}
}

func withReserved(row sqlc.CabalPotSnapshotRow, reserved string) sqlc.CabalPotSnapshotRow {
	row.CashOutReservedMicros = reserved
	return row
}

func TestCabalPots_valuesEveryChainLikeCabalPot(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx().Mint.String()
	rows := []sqlc.CabalPotSnapshotRow{
		potRow(aapl, "100", "50"), potRow(usdcMint, "7", "7"), potRow(evmStock, "5", "2"), potRow(evmCashKey, "3", "3"),
	}
	q := potQueries(potFake{snapshot: rows}, nil)
	var read int
	q.WithListings(func(context.Context, []string, time.Time) (map[string]app.ListingQuote, error) {
		read++
		return map[string]app.ListingQuote{evmStock: evmQuote(q, 0, fixedValue{value: money.MicrosFromUint64(9)})}, nil
	})
	cabals := twoCabals()

	pots, err := q.CabalPots(t.Context(), cabals)
	if err != nil {
		t.Fatal(err)
	}
	if read != 1 || len(pots) != 2 || len(pots[0].Holdings) != 2 {
		t.Fatalf("listing reads %d, pots %d, holdings %+v", read, len(pots), pots[0].Holdings)
	}
	for i, id := range cabals {
		alone, err := q.CabalPot(t.Context(), id, ids.UserID{}, false)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(pots[i], alone) {
			t.Fatalf("pot %d = %+v, want CabalPot's %+v", i, pots[i], alone)
		}
	}
}

func TestWireHeld_refusesMicrosPastInt64(t *testing.T) {
	t.Parallel()
	if _, err := wireHeld(port.HeldList{TotalValueMicros: money.MicrosFromUint64(math.MaxUint64)}); err == nil {
		t.Fatal("wireHeld = nil, want an error")
	}
}
