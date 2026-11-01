package trading_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type cashOnlyLedger struct {
	cashOutLots
	solana uint64
}

func (c *cashOnlyLedger) SolanaPositions(context.Context, ids.CabalID) ([]treasuryport.Position, error) {
	return []treasuryport.Position{{Mint: usdcMint, Units: money.NewBaseUnits(c.solana, 6)}}, nil
}

func (c *cashOnlyLedger) EVMPositions(context.Context, ids.CabalID) ([]treasuryport.ListingPosition, error) {
	return nil, nil
}

const ledgerCash = 40_000_000

func newEVMCashOutWithCash(t *testing.T) (*evmCashOut, *cashOnlyLedger) {
	t.Helper()
	e := newEVMCashOut(t, 50_000_000)
	ledger := &cashOnlyLedger{solana: 900_000_000}
	ledger.cash = map[chain.Network]money.Micros{chain.NetworkBase: money.MicrosFromUint64(ledgerCash)}
	e.ledger = ledger
	e.listing.StockID = e.route.StockID
	e.listings.listings[e.listing.ID] = e.listing
	e.evm.plan.OutAmount, e.evm.plan.MinOut = requotedOut, 302_940_000
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	e.requote()
	e.quote(usdcToken(), aaplxToken(), 0, false)
	e.privy.script(multichain.ActionSucceeded)
	e.mustBuy(t, e.request(), domain.StatusConfirmed)
	e.privy.script(multichain.ActionSucceeded)
	return e, ledger
}

func (e *evmCashOut) wantBridgedCash(t *testing.T, sends int) {
	t.Helper()
	legs := e.legs(t)
	if len(legs) != 1 || legs[0].InMint != "base:"+baseCash || legs[0].Status != string(domain.StatusConfirmed) ||
		legs[0].InAmount < 30_300_000 || legs[0].InAmount > ledgerCash || legs[0].Batch != 1 {
		t.Fatalf("legs = %+v, want one confirmed cash leg covering the amount with the 1%% buffer", legs)
	}
	calls := e.privy.calls()
	if len(calls) != sends+1 || calls[sends].spec.From != chain.NetworkBase ||
		calls[sends].spec.To != chain.NetworkSolana ||
		calls[sends].spec.AmountMicros.String() != strconv.FormatInt(legs[0].InAmount, 10) {
		t.Fatalf("transfers = %+v, want the cash bridged from Base to Solana in full", calls)
	}
}

func TestSellForCashOut_bridgesBackEVMCashWhenItIsTheOnlyHolding(t *testing.T) {
	t.Parallel()
	e, _ := newEVMCashOutWithCash(t)
	sends := len(e.privy.calls())

	if err := e.cashOut(t, 30_000_000); err != nil {
		t.Fatal(err)
	}

	e.wantBridgedCash(t, sends)
	if len(e.lots.asked) != 0 {
		t.Fatalf("asked %v for an EVM sale, want none for cash", e.lots.asked)
	}
	got := e.steps(t, e.swapIDs(t, e.job)[0])
	if got[len(got)-1] != "bridge_back:succeeded:"+strconv.Itoa(cashOutBridgeFee) {
		t.Fatalf("cash leg steps = %v, want the cash bridged back at the observed fee", got)
	}
}

func TestSellForCashOut_aCashLegIsNotSoldAgainOnRedelivery(t *testing.T) {
	t.Parallel()
	e, _ := newEVMCashOutWithCash(t)
	if err := e.cashOut(t, 30_000_000); err != nil {
		t.Fatal(err)
	}
	sends := len(e.privy.calls())

	if err := e.cashOut(t, 30_000_000); err != nil {
		t.Fatal(err)
	}

	if n := len(e.legs(t)); n != 1 || len(e.privy.calls()) != sends {
		t.Fatalf("%d legs and %d transfers after the redelivery, want the one leg and no second bridge", n,
			len(e.privy.calls())-sends)
	}
}

func TestSellForCashOut_aCashLegIsCappedByTheCashOnChain(t *testing.T) {
	t.Parallel()
	e, _ := newEVMCashOutWithCash(t)
	e.cash.balance = money.MicrosFromUint64(10_000_000)

	if err := e.cashOut(t, 30_000_000); err != nil {
		t.Fatal(err)
	}

	if legs := e.legs(t); len(legs) != 1 || legs[0].InAmount != 10_000_000 {
		t.Fatalf("legs = %+v, want the one leg capped at the 10 USDC on chain", legs)
	}
}

func TestSellForCashOut_aWalletWithNoCashOnChainPlansNoCashLeg(t *testing.T) {
	t.Parallel()
	e, _ := newEVMCashOutWithCash(t)
	e.cash.balance = money.Micros{}

	if err := e.cashOut(t, 30_000_000); err != nil {
		t.Fatal(err)
	}

	if n := len(e.legs(t)); n != 0 {
		t.Fatalf("%d legs, want none when the ledger cash has nothing behind it on chain", n)
	}
}

func TestSellForCashOut_aCabalWithoutABridgedTradePlansNoCashLeg(t *testing.T) {
	t.Parallel()
	e := newEVMCashOut(t, 50_000_000)
	ledger := &cashOnlyLedger{solana: 900_000_000}
	ledger.cash = map[chain.Network]money.Micros{chain.NetworkBase: money.MicrosFromUint64(40_000_000)}
	e.ledger = ledger

	if err := e.cashOut(t, 30_000_000); err != nil {
		t.Fatal(err)
	}

	if n := len(e.legs(t)); n != 0 {
		t.Fatalf("%d legs, want none without a route to carry the cash", n)
	}
}

func TestSellForCashOut_aCashLegReadFailureStopsThePlan(t *testing.T) {
	t.Parallel()
	for name, arrange := range map[string]func(*evmCashOut, *cashOnlyLedger){
		"ledger": func(_ *evmCashOut, l *cashOnlyLedger) { l.cashErr = errs.New(errs.CodeDBUnavailable, "test") },
		"chain":  func(e *evmCashOut, _ *cashOnlyLedger) { e.cash.err = errs.New(errs.CodeRPCUnavailable, "test") },
		"wallet": func(e *evmCashOut, _ *cashOnlyLedger) {
			e.treasury.err = errs.New(errs.CodeUpstreamUnavailable, "test")
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e, ledger := newEVMCashOutWithCash(t)
			arrange(e, ledger)

			if err := e.cashOut(t, 30_000_000); err == nil {
				t.Fatal("Handle err = nil, want the failed read")
			}
			if n := len(e.legs(t)); n != 0 {
				t.Fatalf("%d legs, want none", n)
			}
		})
	}
}

func TestLedgerHoldings_listsEVMCashAsAHolding(t *testing.T) {
	t.Parallel()
	ledger := &cashOnlyLedger{}
	ledger.cash = map[chain.Network]money.Micros{
		chain.NetworkRobinhood: money.MicrosFromUint64(7), chain.NetworkBase: money.MicrosFromUint64(5),
		chain.NetworkArbitrum: {},
	}
	holdings := app.LedgerHoldings{
		Ledger:  ledger,
		Catalog: marketfake.NewCatalog(marketfake.Fixtures()...),
		USDC:    usdcToken(),
	}

	got, err := holdings.Positions(t.Context(), ids.CabalID{})

	var cash []app.Holding
	for _, h := range got {
		if h.Symbol == "USDC" || h.Symbol == "USDG" {
			cash = append(cash, h)
		}
	}
	if err != nil || len(cash) != 2 || cash[0].Mint.Address != "base:"+baseCash || cash[0].Units != 5 ||
		cash[1].Mint.Address != "robinhood:"+rhUSDG || cash[1].Units != 7 || cash[1].Mint.Decimals != 6 {
		t.Fatalf("cash holdings = %+v, %v, want Base then Robinhood cash and no zero balance", cash, err)
	}
}

func TestLedgerHoldings_passesOnACashReadFailure(t *testing.T) {
	t.Parallel()
	ledger := &cashOnlyLedger{}
	ledger.cashErr = errs.New(errs.CodeDBUnavailable, "test")
	holdings := app.LedgerHoldings{
		Ledger:  ledger,
		Catalog: marketfake.NewCatalog(marketfake.Fixtures()...),
		USDC:    usdcToken(),
	}

	if _, err := holdings.Positions(t.Context(), ids.CabalID{}); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Positions err = %v, want db_unavailable", err)
	}
}

func TestSellForCashOut_aCashLegWhoseSwapStepCannotBeJournaledStopsTheSale(t *testing.T) {
	t.Parallel()
	e, _ := newEVMCashOutWithCash(t)
	if _, err := e.pool.Exec(t.Context(), `CREATE FUNCTION refuse_swap_step() RETURNS trigger LANGUAGE plpgsql AS
		$$ BEGIN RAISE EXCEPTION 'swap step refused'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(t.Context(), `CREATE TRIGGER refuse_swap_step BEFORE INSERT ON trade_steps
		FOR EACH ROW WHEN (NEW.step = 'swap') EXECUTE FUNCTION refuse_swap_step()`); err != nil {
		t.Fatal(err)
	}

	err := e.cashOut(t, 30_000_000)

	if err == nil {
		t.Fatal("Handle err = nil, want the journal failure")
	}
	if n := len(e.privy.calls()); n != 1 {
		t.Fatalf("%d transfers, want only the earlier buy's bridge", n)
	}
}

func TestSellForCashOut_aCashLegWaitsWhileAnotherSwapHoldsTheTreasuryLock(t *testing.T) {
	t.Parallel()
	e, _ := newEVMCashOutWithCash(t)
	e.mustExec(t, `UPDATE swaps SET status = 'submitted' WHERE source_kind = 'proposal'`)
	e.mustExec(t, `UPDATE evm_swap_locks SET expires_at = $1`, e.clk.Now().Add(time.Hour))
	sends := len(e.privy.calls())

	if err := e.cashOut(t, 30_000_000); err != nil {
		t.Fatal(err)
	}

	if legs := e.legs(t); len(legs) != 1 || legs[0].Status != string(domain.StatusCreated) ||
		len(e.privy.calls()) != sends {
		t.Fatalf("legs = %+v after %d transfers, want the leg created and waiting with no bridge",
			e.legs(t), len(e.privy.calls())-sends)
	}
}
