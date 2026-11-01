package treasury_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func (f fixture) writeOff(w domain.CashWriteOff) error {
	return f.do(func(ctx context.Context, tx db.Tx) error { return f.ledger.WriteOffEVMCash(ctx, tx, w) })
}

func (f fixture) cashTxns(t *testing.T, cabal ids.CabalID, w domain.CashWriteOff) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM cabal_txns WHERE id = $1 AND cabal_id = $2`,
		w.TxnID(), cabal.UUID()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestWriteOffEVMCash_leavesTheLedgerEqualToWhatTheChainHoldsAfterTheReturn(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	f.bridgedOut(t, cabal, 20_000_000, 0)
	swap := f.ids.NewV7()
	back := bridgeRun{f: f, cabal: cabal, swap: swap, step: "bridge_back"}
	back.submitted(t, 17_000_000)
	back.succeeded(t, 17_000_000, 2_500)
	w := domain.CashWriteOff{
		SwapID: swap, CabalID: cabal, Network: chain.NetworkBase, Amount: money.MicrosFromUint64(3_000_000),
	}

	if err := f.writeOff(w); err != nil {
		t.Fatal(err)
	}

	wantBridgePositions(t, f,
		position{Asset: usdcMint, Units: "96997500", Cost: "96997500"},
		position{Asset: baseCash, Units: "0", Cost: "0"},
	)
	var treasury, fees int64
	if err := f.pool.QueryRow(t.Context(), `SELECT
			coalesce(sum(amount) FILTER (WHERE account = 'treasury'), 0),
			coalesce(sum(amount) FILTER (WHERE account = 'fees'), 0)
		FROM cabal_txn_entries WHERE txn_id = $1 AND asset = $2`, w.TxnID(), baseCash).Scan(&treasury, &fees); err != nil {
		t.Fatal(err)
	}
	if treasury != -3_000_000 || fees != 3_000_000 {
		t.Fatalf("write-off moved %d from the treasury and %d to fees, want a balanced 3 USDC", treasury, fees)
	}
}

func TestWriteOffEVMCash_isIdempotentBySwap(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	f.bridgedOut(t, cabal, 20_000_000, 0)
	w := domain.CashWriteOff{
		SwapID: f.ids.NewV7(), CabalID: cabal, Network: chain.NetworkBase, Amount: money.MicrosFromUint64(3_000_000),
	}

	for range 2 {
		if err := f.writeOff(w); err != nil {
			t.Fatal(err)
		}
	}

	wantBridgePositions(t, f,
		position{Asset: usdcMint, Units: "80000000", Cost: "80000000"},
		position{Asset: baseCash, Units: "17000000", Cost: "17000000"},
	)
	if n := f.cashTxns(t, cabal, w); n != 1 {
		t.Fatalf("%d write-off transactions after two runs, want 1", n)
	}
}

func TestWriteOffEVMCash_refusesMoreThanTheLedgerHolds(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	f.bridgedOut(t, cabal, 20_000_000, 0)
	w := domain.CashWriteOff{
		SwapID: f.ids.NewV7(), CabalID: cabal, Network: chain.NetworkBase, Amount: money.MicrosFromUint64(20_000_001),
	}

	if err := f.writeOff(w); err == nil {
		t.Fatal("WriteOffEVMCash err = nil, want the overdraw refused")
	}
	if n := f.cashTxns(t, cabal, w); n != 0 {
		t.Fatalf("%d write-off transactions after a refusal, want none", n)
	}
}

func TestNewCashWriteOffTxn_refusesAnAmountPastInt64(t *testing.T) {
	t.Parallel()
	_, err := domain.NewCashWriteOffTxn(domain.CashWriteOff{
		Network: chain.NetworkBase, Amount: money.MicrosFromUint64(1 << 63),
	})
	if err == nil {
		t.Fatal("NewCashWriteOffTxn err = nil, want the oversize amount refused")
	}
}

func TestWriteOffEVMCash_reportsAFailedCabalLock(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	w := domain.CashWriteOff{
		SwapID: f.ids.NewV7(), CabalID: cabal, Network: chain.NetworkBase, Amount: money.MicrosFromUint64(1),
	}

	err := f.do(func(ctx context.Context, tx db.Tx) error {
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		return f.ledger.WriteOffEVMCash(cancelled, tx, w)
	})

	if err == nil {
		t.Fatal("WriteOffEVMCash err = nil, want the failed lock reported")
	}
}

func TestWriteOffEVMCash_refusesAnAmountPastInt64(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	w := domain.CashWriteOff{
		SwapID: f.ids.NewV7(), CabalID: cabal, Network: chain.NetworkBase, Amount: money.MicrosFromUint64(1 << 63),
	}

	if err := f.writeOff(w); err == nil {
		t.Fatal("WriteOffEVMCash err = nil, want the oversize amount refused")
	}
}
