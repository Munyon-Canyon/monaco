package trading_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const (
	ledgerBaseToken  = "base:0xb200000000000000000000000000000000000d04"
	ledgerHeldUnits  = 100
	insertLedgerLots = `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
VALUES ($1, $2, $3, 60000000, now())`
)

func TestLedgerHoldings_theLastMembersAllStillSeesTheCabalsEVMHolding(t *testing.T) {
	t.Parallel()
	pool, gen := testkit.DB(t), testkit.NewIDs(1)
	cabal := ids.CabalIDFrom(gen.NewV7())
	if _, err := pool.Exec(t.Context(), insertLedgerLots, cabal.UUID(), ledgerBaseToken, ledgerHeldUnits); err != nil {
		t.Fatal(err)
	}
	ledger := treasury.New(module.Deps{Pool: pool, Config: testkit.Config()}).Queries()
	snapshot, err := ledger.CabalPositionsAt(
		t.Context(),
		testkit.NewClock(time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)).Now(),
	)
	if err != nil || len(snapshot) != 0 {
		t.Fatalf("share-weighted snapshot = %+v, %v, want nothing once no shares are left", snapshot, err)
	}
	holdings := app.LedgerHoldings{
		Ledger: ledger, Catalog: marketfake.NewCatalog(marketfake.Fixtures()...), USDC: usdcToken(),
	}

	got, err := holdings.Positions(t.Context(), cabal)

	if err != nil || len(got) != 1 || string(got[0].Mint.Address) != ledgerBaseToken ||
		got[0].Units != ledgerHeldUnits {
		t.Fatalf("Positions = %+v, %v, want the Base holding of %d units", got, err, ledgerHeldUnits)
	}
}

func TestLedgerHoldings_anotherCabalsEVMHoldingIsNotThisCabalsLot(t *testing.T) {
	t.Parallel()
	pool, gen := testkit.DB(t), testkit.NewIDs(2)
	cabal, other := ids.CabalIDFrom(gen.NewV7()), ids.CabalIDFrom(gen.NewV7())
	if _, err := pool.Exec(t.Context(), insertLedgerLots, other.UUID(), ledgerBaseToken, ledgerHeldUnits); err != nil {
		t.Fatal(err)
	}
	holdings := app.LedgerHoldings{
		Ledger:  treasury.New(module.Deps{Pool: pool, Config: testkit.Config()}).Queries(),
		Catalog: marketfake.NewCatalog(marketfake.Fixtures()...), USDC: usdcToken(),
	}

	got, err := holdings.Positions(t.Context(), cabal)

	if err != nil || len(got) != 0 {
		t.Fatalf("Positions = %+v, %v, want nothing for a cabal that holds nothing", got, err)
	}
}

func TestSellForCashOut_aWindDownOfACabalWithNoSharesLeftStillSellsItsEVMLot(t *testing.T) {
	t.Parallel()
	e := newEVMCashOut(t, 50_000_000)
	if _, err := e.pool.Exec(t.Context(), insertLedgerLots, e.cabal.UUID(), e.token(), soldUnits); err != nil {
		t.Fatal(err)
	}
	e.ledger = treasury.New(module.Deps{Pool: e.pool, Config: testkit.Config()}).Queries()

	if err := e.cashOut(t, 30_000_000); err != nil {
		t.Fatal(err)
	}

	legs := e.legs(t)
	if len(legs) != 1 || legs[0].InMint != e.token() || legs[0].InAmount <= 0 || legs[0].InAmount > soldUnits {
		t.Fatalf("legs = %+v, want the Base lot sold although every member's shares are burned", legs)
	}
}
