//go:build faultpoints

package trading_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (e *engineEnv) failedTrade(t *testing.T) (app.ExecuteTrade, uuid.UUID) {
	t.Helper()
	e.jup.SetExecute("req-1", jupiter.ExecuteResult{Status: jupiter.StatusFailed, ErrorCode: 6001})
	cmd := e.buy()
	if err := e.handle(t, e.delivery(t, cmd), cmd); err != nil {
		t.Fatal(err)
	}
	failed := e.onlySwap(t, cmd)
	e.assertSettled(t, failed, domain.StatusFailed, domain.FailureJupiterFailed)
	return cmd, failed
}

func (e *engineEnv) crashRetry(t *testing.T, point faultpoint.Name) uuid.UUID {
	t.Helper()
	cmd, failed := e.failedTrade(t)
	d, retry := e.retry(t, failed, cmd)
	testkit.CrashAt(t, point, func(ctx context.Context) error {
		return e.handler().Handle(actorContext(ctx), d, retry, nil)
	})
	swaps := e.swapsOf(t, cmd.ProposalID)
	if len(swaps) != 2 || swaps[0] != failed {
		t.Fatalf("swaps %v after the restarted retry, want the failed swap and one retry", swaps)
	}
	return swaps[1]
}

func TestFlow12_RetryTrade_CrashAfterCreate(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	retried := e.crashRetry(t, faultpoint.AfterCreate)
	e.sweep(t, recoveryReader{})
	e.assertSettled(t, retried, domain.StatusFailed, domain.FailureNeverSubmitted)
	if n := len(e.jup.Sent("req-2")); n != 0 {
		t.Fatalf("%d executes of the retry, want none: the crash came before the order", n)
	}
}

func TestFlow12_RetryTrade_CrashAfterExecute(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	e.jup.SetExecute("req-2", jupiter.ExecuteResult{Status: jupiter.StatusSuccess, OutAmount: quotedOut})
	retried := e.crashRetry(t, faultpoint.AfterExecute)
	e.sweep(t, landed())
	e.assertSettled(t, retried, domain.StatusConfirmed, "")
	if n := len(e.jup.Sent("req-2")); n != 1 {
		t.Fatalf("%d executes of the retry, want the one before the crash", n)
	}
}
