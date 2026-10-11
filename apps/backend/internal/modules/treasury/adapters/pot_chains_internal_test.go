package adapters

import (
	"context"
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	evmCashKey = "base:0x833589fcd6edb6e08f4c7c32d4f71b54bda02913"
	evmStock   = "base:0xb200000000000000000000000000000000000a01"
	maxMicro   = math.MaxUint64
)

type fixedValue struct {
	value money.Micros
	err   error
}

func chainQueries(quotes map[string]app.ListingQuote, readErr error) *Queries {
	clock := testkit.NewClock(time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC))
	q := &Queries{usdc: usdcMint, clock: clock, pot: potFake{}}
	return q.WithListings(func(context.Context, []string, time.Time) (map[string]app.ListingQuote, error) {
		return quotes, readErr
	})
}

func evmQuote(q *Queries, age time.Duration, v fixedValue) app.ListingQuote {
	return app.ListingQuote{
		ListingID: uuid.UUID{1}, Issuer: "ondo", Symbol: "AAPLon", Network: chain.NetworkBase, Decimals: 0,
		SharesPerToken: big.NewRat(1, 1),
		Price:          &app.Price{Micros: money.MicrosFromUint64(1), ObservedAt: q.clock.Now().Add(-age)},
		Value:          func(*big.Int) (money.Micros, error) { return v.value, v.err },
	}
}

func TestPlace_refusesARowItCannotReadOrAdd(t *testing.T) {
	t.Parallel()
	q := &Queries{usdc: usdcMint}
	for name, rows := range map[string][]heldRow{
		"cash units":      {{asset: evmCashKey, units: "x", cost: "0"}},
		"cash overflow":   {{asset: evmCashKey, units: maxUint64, cost: "0"}, {asset: evmCashKey, units: "1", cost: "0"}},
		"token units":     {{asset: evmStock, units: "x", cost: "0"}},
		"negative tokens": {{asset: evmStock, units: "-1", cost: "0"}},
		"token cost":      {{asset: evmStock, units: "1", cost: "x"}},
	} {
		if _, err := q.byChain(rows); err == nil {
			t.Fatalf("%s: byChain = nil, want an error", name)
		}
	}
	split, err := q.byChain([]heldRow{
		{asset: usdcMint, units: "1", cost: "1"},
		{asset: evmCashKey, units: "7", cost: "7"},
		{asset: evmStock, units: "5", cost: "2"},
	})
	if err != nil || len(split.solana) != 1 || split.evmCash.Uint64() != 7 || len(split.evm) != 1 ||
		split.evm[0].units.Int64() != 5 || split.evm[0].cost.Uint64() != 2 {
		t.Fatalf("byChain = %+v, %v", split, err)
	}
}

func TestValueEVM_refusesAPositionWithoutAFreshReadablePrice(t *testing.T) {
	t.Parallel()
	position := []evmPosition{{key: evmStock, units: big.NewInt(1)}}
	down := errs.New(errs.CodeDBUnavailable, "test")
	unlisted := &Queries{usdc: usdcMint, clock: testkit.NewClock(time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC))}
	if _, _, _, err := unlisted.valueEVM(t.Context(), position); errs.CodeOf(err) != errs.CodePriceUnavailable {
		t.Fatalf("valueEVM without a listing reader = %v, want price_unavailable", err)
	}
	if _, _, _, err := chainQueries(
		nil,
		down,
	).valueEVM(t.Context(), position); errs.CodeOf(
		err,
	) != errs.CodeDBUnavailable {
		t.Fatalf("valueEVM over a failed read = %v, want db_unavailable", err)
	}
	q := chainQueries(nil, nil)
	for name, quote := range map[string]*app.ListingQuote{
		"absent":   nil,
		"no price": {Value: evmQuote(q, 0, fixedValue{}).Value},
		"stale":    ptr(evmQuote(q, multichain.ListingPriceMaxAge+time.Second, fixedValue{})),
		"unvalued": ptr(evmQuote(q, 0, fixedValue{err: down})),
	} {
		quotes := map[string]app.ListingQuote{}
		if quote != nil {
			quotes[evmStock] = *quote
		}
		got := chainQueries(quotes, nil)
		if _, _, _, err := got.valueEVM(t.Context(), position); errs.CodeOf(err) != errs.CodePriceUnavailable {
			t.Fatalf("%s: valueEVM = %v, want price_unavailable", name, err)
		}
	}
	fresh := evmQuote(q, multichain.ListingPriceMaxAge, fixedValue{value: money.MicrosFromUint64(9)})
	held, _, asOf, err := chainQueries(
		map[string]app.ListingQuote{evmStock: fresh},
		nil,
	).valueEVM(t.Context(), position)
	if err != nil || len(held) != 1 || held[0].value.Uint64() != 9 || !asOf.Equal(fresh.Price.ObservedAt) {
		t.Fatalf("valueEVM at exactly five minutes = %+v, %v, %v, want it valued and as of the sample", held, asOf, err)
	}
}

func ptr[T any](v T) *T { return &v }

func TestOffSolana_refusesTotalsPastUint64(t *testing.T) {
	t.Parallel()
	q := chainQueries(nil, nil)
	big9 := map[string]app.ListingQuote{evmStock: evmQuote(q, 0, fixedValue{value: money.MicrosFromUint64(maxMicro)})}
	one := money.MicrosFromUint64(1)
	position := []evmPosition{{key: evmStock, units: big.NewInt(1)}}
	for name, c := range map[string]struct {
		q     *Queries
		split chainRows
	}{
		"cash and transit": {q, chainRows{evmCash: money.MicrosFromUint64(maxMicro)}},
		"unpriced":         {chainQueries(nil, nil), chainRows{evm: position}},
		"holdings":         {chainQueries(big9, nil), chainRows{evmCash: one, evm: position}},
	} {
		if _, err := c.q.offSolana(t.Context(), one, c.split); err == nil {
			t.Fatalf("%s: offSolana = nil, want an error", name)
		}
	}
}

type valueRows []sqlc.CabalValueRowsRow

func (r valueRows) CabalValueRows(context.Context, uuid.UUID) ([]sqlc.CabalValueRowsRow, error) {
	return r, nil
}

func valueRow(asset, units, transit string) sqlc.CabalValueRowsRow {
	return sqlc.CabalValueRowsRow{
		CashOutReservedMicros: "0", InTransitMicros: transit, Asset: pgtype.Text{String: asset, Valid: asset != ""},
		Units: units, CostBasisMicros: "0",
	}
}

func TestPotValue_refusesEVMRowsItCannotReadOrValue(t *testing.T) {
	t.Parallel()
	for name, rows := range map[string]valueRows{
		"evm cash":    {valueRow(evmCashKey, "x", "0")},
		"transit":     {valueRow("", "0", "x")},
		"unpriced":    {valueRow(evmStock, "1", "0")},
		"past uint64": {valueRow(usdcMint, maxUint64, "0"), valueRow(evmCashKey, "1", "0")},
	} {
		q := chainQueries(nil, nil)
		q.transit = rows
		if _, err := q.PotValue(t.Context(), ids.CabalID{}); err == nil {
			t.Fatalf("%s: PotValue = nil, want an error", name)
		}
	}
	q := chainQueries(nil, nil)
	q.transit = valueRows{valueRow(usdcMint, "3", "4"), valueRow(evmCashKey, "5", "4")}
	if got, err := q.PotValue(t.Context(), ids.CabalID{}); err != nil || got.Uint64() != 12 {
		t.Fatalf("PotValue = %v, %v, want 3 Solana + 5 EVM cash + 4 in transit", got, err)
	}
}

func TestEVMCashKey_isBaseUSDC(t *testing.T) {
	t.Parallel()
	if evmCashKey != "base:"+chain.NetworkBase.Cash().Address {
		t.Fatalf("evmCashKey = %s, want Base USDC", evmCashKey)
	}
}

func snapshotRow(cabal uuid.UUID, asset, units, transit string) sqlc.CabalPositionSnapshotsAtRow {
	return sqlc.CabalPositionSnapshotsAtRow{
		CabalID: cabal, Asset: asset, Units: units, CostBasisMicros: "0", ShareUnits: "1", InTransitMicros: transit,
	}
}

func TestCabalPositionsAt_carriesEVMCashListingsAndMoneyInTransit(t *testing.T) {
	t.Parallel()
	cabal := uuid.UUID{7}
	q := &Queries{usdc: usdcMint}
	q.history = historicalStore{snapshots: []sqlc.CabalPositionSnapshotsAtRow{
		snapshotRow(cabal, usdcMint, "80", "5"), snapshotRow(cabal, evmCashKey, "3", "5"),
		snapshotRow(cabal, evmCashKey, "4", "5"), snapshotRow(cabal, evmStock, "500000000000000000000", "5"),
	}}
	got, err := q.CabalPositionsAt(t.Context(), time.Time{})
	if err != nil || len(got) != 1 || len(got[0].Holdings) != 1 || got[0].EVMCash.Uint64() != 7 ||
		got[0].InTransit.Uint64() != 5 || len(got[0].Listings) != 1 ||
		got[0].Listings[0].Units.String() != "500000000000000000000" {
		t.Fatalf("CabalPositionsAt = %+v, %v, want Solana USDC, 7 EVM cash, 5 in transit and one listing", got, err)
	}
}

func TestCabalPositionsAt_refusesEVMRowsAndTransitItCannotRead(t *testing.T) {
	t.Parallel()
	cabal := uuid.UUID{7}
	for name, rows := range map[string][]sqlc.CabalPositionSnapshotsAtRow{
		"transit sum": {snapshotRow(cabal, usdcMint, "1", "x")},
		"evm cash":    {snapshotRow(cabal, evmCashKey, "x", "0")},
		"evm cash sum": {
			snapshotRow(cabal, evmCashKey, maxUint64, "0"), snapshotRow(cabal, evmCashKey, "1", "0"),
		},
		"token units":     {snapshotRow(cabal, evmStock, "x", "0")},
		"negative tokens": {snapshotRow(cabal, evmStock, "-1", "0")},
	} {
		q := &Queries{usdc: usdcMint, history: historicalStore{snapshots: rows}}
		if _, err := q.CabalPositionsAt(t.Context(), time.Time{}); err == nil {
			t.Fatalf("%s: CabalPositionsAt = nil, want an error", name)
		}
	}
}

func TestSolanaPositions_returnsTheSolanaPositionsAndSkipsEVMRows(t *testing.T) {
	t.Parallel()
	q := newTestQueries(stakeStore{positions: []sqlc.CabalPositionsRow{
		{Asset: usdcMint, Units: "5", CostBasisMicros: "5"},
		{Asset: evmCashKey, Units: "7", CostBasisMicros: "7"},
		{Asset: evmStock, Units: "500000000000000000000", CostBasisMicros: "15"},
	}}, nil)
	got, err := q.SolanaPositions(t.Context(), ids.CabalID{})
	if err != nil || len(got) != 1 || string(got[0].Mint) != usdcMint || got[0].Units.Uint64() != 5 {
		t.Fatalf("Positions = %+v, %v, want only the Solana USDC row and no decode error", got, err)
	}
}
