package trading_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (e *routeEnv) submittedWithoutReceipt(t *testing.T) uuid.UUID {
	t.Helper()
	e.privy.script(multichain.ActionSucceeded)
	view := e.mustBuy(t, e.request(), domain.StatusSubmitted)
	return view.ID.UUID()
}

func (e *routeEnv) signature(t *testing.T, swap uuid.UUID) chain.Signature {
	t.Helper()
	var sig string
	if err := e.pool.QueryRow(t.Context(), `SELECT tx_signature FROM swaps WHERE id = $1`, swap).
		Scan(&sig); err != nil {
		t.Fatal(err)
	}
	return chain.Signature(sig)
}

func (e *routeEnv) swapStep(t *testing.T, swap uuid.UUID) string {
	t.Helper()
	var status string
	if err := e.pool.QueryRow(t.Context(), `SELECT status FROM trade_steps WHERE trade_id = $1 AND step = 'swap'`, swap).
		Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func (e *routeEnv) wantParked(t *testing.T, swap uuid.UUID, wantCode string) {
	t.Helper()
	if status, code := e.parked(t, swap); status != string(domain.StatusSubmitted) || code != wantCode {
		t.Fatalf("swap %s parked %q, want submitted and parked %q", status, code, wantCode)
	}
}

func (e *routeEnv) parkedWithoutReceipt(t *testing.T) uuid.UUID {
	t.Helper()
	swap := e.submittedWithoutReceipt(t)
	e.clk.Advance(app.SwapSweepAge + time.Second)
	if _, err := e.tick(t.Context(), t); err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(11 * time.Minute)
	if _, err := e.tick(t.Context(), t); err != nil {
		t.Fatal(err)
	}
	e.wantParked(t, swap, app.ParkedSwapReceiptMissing)
	return swap
}

func TestEVMTrades_parksAndAlertsOnASwapWithNoReceiptAfterTenMinutes(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.submittedWithoutReceipt(t)
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))

	e.clk.Advance(app.SwapSweepAge + time.Second)
	if report, err := e.tick(ctx, t); err != nil || report.Scanned != 1 {
		t.Fatalf("young Tick = %+v, %v, want the pending swap resumed", report, err)
	}
	e.wantParked(t, swap, "")

	e.clk.Advance(10 * time.Minute)
	if _, err := e.tick(ctx, t); err != nil {
		t.Fatal(err)
	}
	e.wantParked(t, swap, app.ParkedSwapReceiptMissing)
	for range 2 {
		if report, err := e.tick(ctx, t); err != nil || report.Scanned != 0 {
			t.Fatalf("later Tick = %+v, %v, want the parked swap skipped", report, err)
		}
	}
	alerts := logs.Lines(observability.TradingRoutedSwapParked.Name, "level", "swap_id", "cabal_id", "code")
	want := "ERROR " + swap.String() + " " + e.cabal.String() + " swap_receipt_missing"
	if len(alerts) != 1 || alerts[0] != want {
		t.Fatalf("park alerts = %q, want one %q", alerts, want)
	}
}

func TestForceResolveSwap_closesAParkedRoutedSwapFromItsReceipt(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.parkedWithoutReceipt(t)
	handler := app.NewForceResolveSwapHandler(e.uow, e.pool, e.clk, e.hints)

	view, err := handler.Handle(t.Context(), app.ForceResolveSwap{
		Signature: e.signature(t, swap), To: domain.StatusConfirmed, OutAmount: money.NewBaseUnits(filledOut, 8),
		Reason: "receipt found on the explorer", Actor: "system:alice", IdempotencyKey: "k1",
	})
	if err != nil || view.Status != domain.StatusConfirmed {
		t.Fatalf("Handle = %+v, %v, want the parked routed swap confirmed", view, err)
	}
	if step := e.swapStep(t, swap); step != "succeeded" {
		t.Fatalf("swap step is %s, want succeeded", step)
	}
}

func TestForceResolveSwap_stillRefusesAnUnparkedRoutedSwap(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.submittedWithoutReceipt(t)
	handler := app.NewForceResolveSwapHandler(e.uow, e.pool, e.clk, e.hints)

	_, err := handler.Handle(t.Context(), app.ForceResolveSwap{
		Signature: e.signature(t, swap), To: domain.StatusFailed, Reason: "no receipt", Actor: "system:alice",
		IdempotencyKey: "k1",
	})
	if err == nil {
		t.Fatal("Handle succeeded on a routed swap that is not parked")
	}
}

func TestResolveParkedSwap_failsAParkedRoutedSwapWithNoReceiptAndFreesItsCash(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.parkedWithoutReceipt(t)

	if err := e.resolveParked(t, swap, app.ParkedFail); err != nil {
		t.Fatal(err)
	}
	if status, _, _, _ := e.row(t, swap); status != string(domain.StatusFailed) {
		t.Fatalf("swap is %s, want failed", status)
	}
	if step := e.swapStep(t, swap); step != "failed" {
		t.Fatalf("swap step is %s, want failed", step)
	}
	if e.failure(t, swap) != string(domain.FailureForceResolved) {
		t.Fatalf("failure code is %s, want force_resolved", e.failure(t, swap))
	}
}

func TestRouteExec_twoBuysFromOneCabalSendOneAfterTheOther(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionSucceeded, multichain.ActionSucceeded)
	e.requote()
	first := e.mustBuy(t, e.request(), domain.StatusSubmitted)
	afterFirst := e.evm.keys()

	second, err := e.buy(t.Context(), e.request())
	if err != nil || second.Status != domain.StatusCreated {
		t.Fatalf("second Buy = %+v, %v, want it held back while the first swap is unconfirmed", second, err)
	}
	if got := e.evm.keys(); len(got) != len(afterFirst) {
		t.Fatalf("sends %v after the second buy, want only the first swap's %v", got, afterFirst)
	}

	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	e.resume(t, first.ID, domain.StatusConfirmed)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	e.requote()
	e.resume(t, second.ID, domain.StatusConfirmed)

	keys := e.evm.keys()
	if len(keys) != 4 || keys[0] != "trade:"+first.ID.String()+":swap:approve:0" ||
		keys[1] != "trade:"+first.ID.String()+":swap" || keys[2] != "trade:"+second.ID.String()+":swap:approve:0" ||
		keys[3] != "trade:"+second.ID.String()+":swap" {
		t.Fatalf("sends = %v, want the first swap's approval and swap, then the second's", keys)
	}
}

func TestEVMTrades_skipsALeasedSwapUntilTheLeaseExpires(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	view := e.mustBuy(t, e.request(), domain.StatusCreated)
	e.clk.Advance(app.SwapSweepAge + time.Second)
	e.mustExec(t, `UPDATE swaps SET lease_holder = $2, lease_until = $3 WHERE id = $1`,
		view.ID.UUID(), e.ids.NewV7(), e.clk.Now().Add(time.Minute))
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}

	if report, err := e.tick(t.Context(), t); err != nil || report.Scanned != 0 {
		t.Fatalf("Tick = %+v, %v, want the leased swap skipped", report, err)
	}
	if err := e.exec().Resume(actorContext(t.Context()), ids.SwapIDFrom(view.ID.UUID())); err != nil {
		t.Fatal(err)
	}
	if n := len(e.privy.calls()); n != 1 || len(e.evm.keys()) != 0 {
		t.Fatalf(
			"%d transfers and sends %v while another runner holds the lease, want the bridge only",
			n,
			e.evm.keys(),
		)
	}

	e.clk.Advance(2 * time.Minute)
	if report, err := e.tick(t.Context(), t); err != nil || report.Scanned != 1 {
		t.Fatalf("Tick after the lease expired = %+v, %v, want the swap resumed", report, err)
	}
	if status, _, _, _ := e.row(t, view.ID.UUID()); status != string(domain.StatusConfirmed) {
		t.Fatalf("swap is %s, want confirmed", status)
	}
}
