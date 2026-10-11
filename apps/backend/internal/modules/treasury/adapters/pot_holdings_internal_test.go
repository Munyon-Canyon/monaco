package adapters

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestAddOffSolana_refusesAFailedTransitReadAndCashPastUint64(t *testing.T) {
	t.Parallel()
	q := chainQueries(nil, nil)
	q.pot = potFake{transitErr: errs.New(errs.CodeDBUnavailable, "test")}
	pot := &potBuilder{port.CabalPot{CabalID: ids.CabalID{}}}
	if err := q.addOffSolana(t.Context(), pot, nil, nil, chainRows{}); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("addOffSolana over a failed transit read = %v, want db_unavailable", err)
	}
	q.pot = transitFake{raw: "x"}
	if err := q.addOffSolana(t.Context(), pot, nil, nil, chainRows{}); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("addOffSolana over an unreadable transit sum = %v, want decode_failed", err)
	}
	q.pot = transitFake{raw: "1"}
	full := &potBuilder{port.CabalPot{CashMicros: money.MicrosFromUint64(maxMicro)}}
	if err := q.addOffSolana(t.Context(), full, nil, nil, chainRows{evmCash: money.MicrosFromUint64(1)}); err == nil {
		t.Fatal("addOffSolana past uint64 on EVM cash = nil")
	}
	full = &potBuilder{port.CabalPot{CashMicros: money.MicrosFromUint64(maxMicro)}}
	if err := q.addOffSolana(t.Context(), full, nil, nil, chainRows{}); err == nil {
		t.Fatal("addOffSolana past uint64 on transit = nil")
	}
	unpriced := chainRows{evm: []evmPosition{{key: evmStock, units: big.NewInt(1)}}}
	if err := q.addOffSolana(t.Context(), pot, nil, nil, unpriced); errs.CodeOf(err) != errs.CodePriceUnavailable {
		t.Fatalf("addOffSolana with an unpriced EVM position = %v, want price_unavailable", err)
	}
}

type transitFake struct {
	potFake
	raw string
}

func (f transitFake) CabalInTransitMicros(context.Context, uuid.UUID) (string, error) {
	return f.raw, nil
}

func TestStockGroupAdd_refusesSumsPastTheirTypes(t *testing.T) {
	t.Parallel()
	q := chainQueries(nil, nil)
	held := evmHolding{
		evmPosition: evmPosition{key: evmStock, units: big.NewInt(1), cost: money.MicrosFromUint64(1)},
		quote:       evmQuote(q, 0, fixedValue{}),
	}
	for name, c := range map[string]struct {
		holding port.Holding
		value   uint64
	}{
		"value": {port.Holding{ValueMicros: money.MicrosFromUint64(maxMicro)}, 1},
		"cost":  {port.Holding{CostBasisMicros: money.MicrosFromUint64(maxMicro)}, 0},
		"pnl":   {port.Holding{}, maxMicro},
	} {
		h := held
		h.value = money.MicrosFromUint64(c.value)
		g := &stockGroup{shares: new(big.Rat)}
		if err := g.add(&c.holding, h); err == nil {
			t.Fatalf("%s: add = nil, want an error", name)
		}
	}
	pot := &potBuilder{port.CabalPot{Holdings: []port.Holding{{ValueMicros: money.MicrosFromUint64(maxMicro)}}}}
	solana := []port.Position{{Mint: chain.SolanaAddress("XsbE"), Units: money.NewBaseUnits(1, 0)}}
	quote := evmQuote(q, 0, fixedValue{value: money.MicrosFromUint64(1)})
	quote.Asset = &app.ListedAsset{Mint: "XsbE"}
	merged := chainQueries(map[string]app.ListingQuote{evmStock: quote}, nil)
	split := chainRows{evm: []evmPosition{{key: evmStock, units: big.NewInt(1)}}}
	if err := merged.mergeEVM(t.Context(), pot, solana, nil, split); err == nil {
		t.Fatal("mergeEVM past uint64 = nil")
	}
	if len(pot.Holdings[0].Listings) != 0 {
		t.Fatalf("listings = %+v, want no Solana entry when the Solana listing is unknown", pot.Holdings[0].Listings)
	}
}

func TestFormatShares_roundsDownToFourPlaces(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"5/4": "1.2500", "1/3": "0.3333", "2": "2.0000"} {
		r, _ := new(big.Rat).SetString(in)
		if got := formatShares(r); got != want {
			t.Fatalf("formatShares(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestCabalPot_refusesEVMRowsItCannotReadOrValue(t *testing.T) {
	t.Parallel()
	for name, asset := range map[string][2]string{
		"evm cash": {evmCashKey, "x"},
		"unpriced": {evmStock, "1"},
	} {
		q := potQueries(potFake{snapshot: []sqlc.CabalPotSnapshotRow{potRow(asset[0], asset[1], "0")}}, nil)
		if _, err := q.CabalPot(t.Context(), ids.CabalID{}, potViewer(), false); err == nil {
			t.Fatalf("%s: CabalPot = nil, want an error", name)
		}
	}
}

func TestMergeEVM_groupsByStockAndTakesTheOldestPrice(t *testing.T) {
	t.Parallel()
	q := chainQueries(nil, nil)
	older := evmQuote(q, time.Minute, fixedValue{value: money.MicrosFromUint64(5)})
	older.Asset = &app.ListedAsset{Mint: "AAPLmint", Symbol: "AAPLx", DisplayName: "Apple", Kind: "equity"}
	merged := chainQueries(map[string]app.ListingQuote{evmStock: older}, nil)
	pot := &potBuilder{port.CabalPot{
		PricesAsOf: q.clock.Now(), Holdings: []port.Holding{{Symbol: "TSLAx", ValueMicros: money.MicrosFromUint64(7)}},
	}}
	solana := []port.Position{{Mint: chain.SolanaAddress("TSLAmint"), Units: money.NewBaseUnits(1, 0)}}
	split := chainRows{evm: []evmPosition{{key: evmStock, units: big.NewInt(1)}}}
	if err := merged.mergeEVM(t.Context(), pot, solana, nil, split); err != nil {
		t.Fatal(err)
	}
	if len(pot.Holdings) != 2 || pot.Holdings[0].ValueMicros.Uint64() != 7 || pot.Holdings[1].Symbol != "AAPLx" ||
		pot.Holdings[1].DisplayName != "Apple" || pot.Holdings[1].ValueMicros.Uint64() != 5 ||
		!pot.PricesAsOf.Equal(older.Price.ObservedAt) {
		t.Fatalf("pot = %+v, want TSLAx untouched, an Apple holding from Base, and prices as of the Base sample", pot)
	}
}
