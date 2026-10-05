//go:build faultpoints

package trading_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (e *engineEnv) crashThenRedeliver(t *testing.T, point faultpoint.Name, cmd app.ExecuteTrade) bus.Delivery {
	t.Helper()
	d := e.delivery(t, cmd)
	testkit.CrashAt(t, point, func(ctx context.Context) error {
		return e.handler().Handle(actorContext(ctx), d, cmd, nil)
	})
	return d
}

func (e *engineEnv) crashBeforeCommit(t *testing.T, skip int, d bus.Delivery, cmd app.ExecuteTrade) {
	t.Helper()
	armed := faultpoint.ArmedAfter(actorContext(t.Context()), faultpoint.BeforeCommit, skip)
	crashed := func() (p any) {
		defer func() { p = recover() }()
		_ = e.handler().Handle(armed, d, cmd, nil)
		return nil
	}()
	if crashed != (faultpoint.Crash{Name: faultpoint.BeforeCommit}) {
		t.Fatalf("the run ended with %v, want a crash before commit %d", crashed, skip+1)
	}
	if err := e.handle(t, d, cmd); err != nil {
		t.Fatalf("the redelivery after the crash: %v", err)
	}
}

func (e *engineEnv) assertOneSwap(t *testing.T, cmd app.ExecuteTrade, status string, executes int) {
	t.Helper()
	swaps := e.swapsOf(t, cmd.ProposalID)
	if len(swaps) != 1 {
		t.Fatalf("%d swaps after the redelivery, want exactly one: a crash never doubles a trade", len(swaps))
	}
	if got, _, _, _ := e.row(t, swaps[0]); got != status || len(e.jup.Sent("req-1")) != executes {
		t.Fatalf("swap %s with %d executes, want %s with %d", got, len(e.jup.Sent("req-1")), status, executes)
	}
	e.assertAtMostOneTerminalEvent(t)
}

func TestTradeEngine_CrashAfterCreate_RedeliveryLeavesTheCreatedRowToTheSweeper(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	cmd := e.buy()
	d := e.crashThenRedeliver(t, faultpoint.AfterCreate, cmd)
	e.assertOneSwap(t, cmd, "created", 0)
	if !e.recorded(t, d) {
		t.Fatal("the redelivery acked without recording the delivery, so the event never shows as handled")
	}
}

func TestTradeEngine_CrashAfterExecute_RedeliveryNeverSendsTheSwapAgain(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	e.jup.SetExecute("req-1", jupiter.ExecuteResult{Status: jupiter.StatusSuccess, OutAmount: 21_000_000})
	cmd := e.buy()
	e.crashThenRedeliver(t, faultpoint.AfterExecute, cmd)
	e.assertOneSwap(t, cmd, "submitted", 1)
}

func TestTradeEngine_CrashBeforeTheDeliveryCommit_RedeliveryKeepsTheOneConfirmedSwap(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	cmd := e.buy()
	e.crashBeforeCommit(t, 3, e.delivery(t, cmd), cmd)
	e.assertOneSwap(t, cmd, "confirmed", 1)
}

func TestTradeEngine_CrashBeforeTheBlockCommit_RedeliveryBlocksOnce(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	e.seedCabal(cabal.StatusBanned, 100)
	cmd := e.buy()
	d := e.delivery(t, cmd)
	e.crashBeforeCommit(t, 0, d, cmd)
	if n := len(e.blocked(t, cmd.ProposalID)); n != 1 || !e.recorded(t, d) {
		t.Fatalf("%d trade.blocked, recorded %v; want one block and the delivery after the redelivery",
			n, e.recorded(t, d))
	}
}
