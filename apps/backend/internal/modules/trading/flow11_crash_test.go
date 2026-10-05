//go:build faultpoints

package trading_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"

	busevents "github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func landed() recoveryReader {
	return recoveryReader{
		statuses: []app.SigStatus{{State: app.SigFinalized}},
		inbound:  money.NewBaseUnits(quotedOut, aaplxToken().Decimals),
	}
}

func (e *engineEnv) sweep(t *testing.T, r recoveryReader) {
	t.Helper()
	e.clk.Advance(app.SwapSweepAge + time.Second)
	if _, err := app.NewSwapSweeper(e.uow, e.pool, e.clk, r, e.hints, app.DefaultSweepTiming()).
		Tick(actorContext(t.Context())); err != nil {
		t.Fatalf("the sweep after the restart: %v", err)
	}
}

func (e *engineEnv) assertSettled(t *testing.T, swap uuid.UUID, status domain.Status, failure domain.FailureCode) {
	t.Helper()
	got, _, _, _ := e.row(t, swap)
	if got != string(status) || e.terminalEvents(t, swap) != 1 {
		t.Fatalf("swap %s with %d terminal events, want %s with exactly one", got, e.terminalEvents(t, swap), status)
	}
	if failure != "" {
		if code := e.payload(t, swap, "trade.failed")["failure_code"]; code != string(failure) {
			t.Fatalf("trade.failed failure_code = %v, want %s", code, failure)
		}
	}
}

func (e *engineEnv) onlySwap(t *testing.T, cmd app.ExecuteTrade) uuid.UUID {
	t.Helper()
	swaps := e.swapsOf(t, cmd.ProposalID)
	if len(swaps) != 1 {
		t.Fatalf("%d swaps for the proposal, want one: a crash never doubles a trade", len(swaps))
	}
	return swaps[0]
}

func (e *engineEnv) retry(t *testing.T, of uuid.UUID, cmd app.ExecuteTrade) (bus.Delivery, app.ExecuteTrade) {
	t.Helper()
	ev := retryOf(of, cmd, 100)
	var id uuid.UUID
	err := e.uow.Do(actorContext(t.Context()), func(ctx context.Context, tx db.Tx) error {
		return tx.Events.Append(ctx, ev)
	})
	if err == nil {
		err = e.pool.QueryRow(t.Context(), `SELECT id FROM events WHERE type = 'trade.retry_requested'
			AND aggregate_id = $1 ORDER BY id DESC LIMIT 1`, of).Scan(&id)
	}
	if err != nil {
		t.Fatal(err)
	}
	next := swapTx()
	next[len(next)-2] = 1
	e.jup.SetOrder(jupiterMint(usdcToken()), jupiterMint(aaplxToken()),
		jupiter.Order{RequestID: "req-2", Transaction: next})
	cmd.Retry = &app.Retry{Of: ids.SwapIDFrom(of), SlippageBps: 100}
	return bus.Delivery{Handler: retryHandler, EventID: ids.EventIDFrom(id), At: e.clk.Now()}, cmd
}

func TestFlow11_ExecuteTrade_CrashAfterCreate(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	cmd := e.buy()
	e.crashThenRedeliver(t, faultpoint.AfterCreate, cmd)
	created := e.onlySwap(t, cmd)
	e.sweep(t, recoveryReader{})
	e.assertSettled(t, created, domain.StatusFailed, domain.FailureNeverSubmitted)

	d, retry := e.retry(t, created, cmd)
	if err := e.handle(t, d, retry); err != nil {
		t.Fatalf("RetryTrade after the sweep: %v", err)
	}
	swaps := e.swapsOf(t, cmd.ProposalID)
	if len(swaps) != 2 {
		t.Fatalf("%d swaps after the retry, want the failed one and its retry", len(swaps))
	}
	e.assertSettled(t, swaps[1], domain.StatusConfirmed, "")
}

func TestFlow11_ExecuteTrade_CrashAfterSign(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	cmd := e.buy()
	e.crashThenRedeliver(t, faultpoint.AfterSign, cmd)
	e.sweep(t, recoveryReader{statuses: []app.SigStatus{{State: app.SigNotFound}}})
	e.assertSettled(t, e.onlySwap(t, cmd), domain.StatusFailed, domain.FailureBlockhashExpired)
}

func TestFlow11_ExecuteTrade_CrashAfterExecute(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	e.jup.SetExecute("req-1", jupiter.ExecuteResult{Status: jupiter.StatusSuccess, OutAmount: quotedOut})
	cmd := e.buy()
	e.crashThenRedeliver(t, faultpoint.AfterExecute, cmd)
	e.sweep(t, landed())
	swap := e.onlySwap(t, cmd)
	e.assertSettled(t, swap, domain.StatusConfirmed, "")
	if n := len(e.jup.Sent("req-1")); n != 1 {
		t.Fatalf("%d executes, want the one before the crash", n)
	}
}

func TestFlow11_ExecuteTrade_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	cmd := e.buy()
	e.crashBeforeCommit(t, 3, e.delivery(t, cmd), cmd)
	e.sweep(t, landed())
	e.assertSettled(t, e.onlySwap(t, cmd), domain.StatusConfirmed, "")
}

func TestFlow11_ExecuteTrade_CrashAfterPublish(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	cmd := e.buy()
	if err := e.handle(t, e.delivery(t, cmd), cmd); err != nil {
		t.Fatal(err)
	}
	swap := e.onlySwap(t, cmd)
	e.assertSettled(t, swap, domain.StatusConfirmed, "")
	b := testkit.NATS(t)
	relay := bus.NewRelay(b.Conn, db.NewOutbox(e.pool, e.clk), e.uow.Signal(), e.clk)
	testkit.CrashAt(t, faultpoint.AfterPublish, func(ctx context.Context) error {
		relay.Once(ctx)
		return nil
	})
	for e.count(t, `SELECT count(*) FROM events WHERE published_at IS NULL`) > 0 {
		relay.Once(t.Context())
	}
	stream, err := b.JS.Stream(t.Context(), b.Events)
	if err != nil {
		t.Fatal(err)
	}
	for _, typ := range []busevents.Type{busevents.TypeProposalPassed, busevents.TypeTradeConfirmed} {
		subject := b.Conn.Subject(typ.Subject())
		info, err := stream.Info(t.Context(), jetstream.WithSubjectFilter(subject))
		if err != nil || info.State.Subjects[subject] != 1 {
			t.Fatalf("%s on the stream = %v, %v; want exactly one after the republish", typ, info, err)
		}
	}
}
