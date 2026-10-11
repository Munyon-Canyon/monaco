package trading_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const evmTreasury = "0x00000000000000000000000000000000000000aa"

type bridgeEnv struct {
	stepEnv
	clk   *testkit.Clock
	privy *privyFake
	leg   app.BridgeLeg
}

func newBridgeEnv(t *testing.T) bridgeEnv {
	t.Helper()
	e := bridgeEnv{stepEnv: newStepEnv(t), clk: testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))}
	e.journal = e.journalOn(steppingClock{e.clk})
	e.privy = newPrivyFake()
	e.leg = app.BridgeLeg{
		Ref: e.ref, WalletID: treasuryWallet, Fee: money.MicrosFromUint64(7_500),
		Spec: multichain.TransferSpec{
			From: chain.NetworkSolana, To: chain.NetworkBase, Destination: evmTreasury,
			AmountMicros: money.MicrosFromUint64(10_000_000),
		},
	}
	return e
}

func (e stepEnv) journalOn(c clock.Clock) app.Journal {
	return app.Journal{UoW: db.New(e.pool, e.ids, c), Reads: e.pool, Clock: c, IDs: e.ids}
}

func (e bridgeEnv) run(ctx context.Context) (domain.StepStatus, error) {
	return app.Bridge{Journal: e.journal, Actions: e.privy, Poll: app.DefaultPoll()}.Run(actorContext(ctx), e.leg)
}

func (e bridgeEnv) key() string { return "trade:" + e.ref.SwapID.String() + ":bridge_out" }

func (e bridgeEnv) wantRun(ctx context.Context, t *testing.T, want domain.StepStatus) {
	t.Helper()
	if got, err := e.run(ctx); err != nil || got != want {
		t.Fatalf("Run = %s, %v, want %s", got, err, want)
	}
}

func (e bridgeEnv) wantOneTransfer(t *testing.T, calls int) {
	t.Helper()
	got := e.privy.calls()
	if len(got) != calls || e.privy.actions() != 1 {
		t.Fatalf("transfers = %+v (%d actions), want %d calls on one action", got, e.privy.actions(), calls)
	}
	for _, c := range got {
		if c.key != e.key() || c.wallet != treasuryWallet || c.spec != e.leg.Spec {
			t.Fatalf("transfer %+v, want the leg's spec from %s under key %s", c, treasuryWallet, e.key())
		}
	}
}

func TestBridge_aTransferThatLandsWalksPendingSubmittedSucceededOnce(t *testing.T) {
	t.Parallel()
	e := newBridgeEnv(t)
	e.privy.script(multichain.ActionPending, multichain.ActionSucceeded)

	e.wantRun(t.Context(), t, domain.StepSucceeded)
	e.wantRun(t.Context(), t, domain.StepSucceeded)

	e.wantOneTransfer(t, 1)
	if e.privy.polls != 2 {
		t.Fatalf("%d action polls, want 2: the finished second run asks Privy nothing", e.privy.polls)
	}
	wantEvents(t, e.events(t),
		e.event(domain.StepPending, "", 0),
		e.event(domain.StepSubmitted, "privy-action-1", 0),
		e.event(domain.StepSucceeded, "privy-action-1", 7_500),
	)
}

func TestBridge_aCrashAfterTheTransferResumesUnderTheSameKeyWithOneAction(t *testing.T) {
	t.Parallel()
	e := newBridgeEnv(t)
	ctx, crash := context.WithCancel(t.Context())
	e.privy.afterTransfer = crash
	e.privy.script(multichain.ActionSucceeded)

	if _, err := e.run(ctx); err == nil {
		t.Fatal("the crashed run returned no error")
	}
	if s := e.step(t); s.Status != domain.StepPending {
		t.Fatalf("after the crash step = %+v, want pending with no action stored", s)
	}
	e.wantRun(t.Context(), t, domain.StepSucceeded)

	e.wantOneTransfer(t, 2)
	wantEvents(t, e.events(t),
		e.event(domain.StepPending, "", 0),
		e.event(domain.StepSubmitted, "privy-action-1", 0),
		e.event(domain.StepSucceeded, "privy-action-1", 7_500),
	)
}

func TestBridge_aCrashAfterTheActionLandsResumesWithoutASecondTransfer(t *testing.T) {
	t.Parallel()
	e := newBridgeEnv(t)
	ctx, crash := context.WithCancel(t.Context())
	e.privy.script(multichain.ActionSucceeded, multichain.ActionSucceeded)
	e.privy.afterAction = crash

	if _, err := e.run(ctx); err == nil {
		t.Fatal("the crashed run returned no error")
	}
	if s := e.step(t); s.Status != domain.StepSubmitted {
		t.Fatalf("after the crash step = %+v, want submitted", s)
	}
	e.wantRun(t.Context(), t, domain.StepSucceeded)

	e.wantOneTransfer(t, 1)
	if n := len(e.events(t)); n != 3 {
		t.Fatalf("%d step events, want pending, submitted and succeeded once each", n)
	}
}

func TestBridge_anActionStillPendingAfterTheBudgetIsLeftSubmittedForTheResume(t *testing.T) {
	t.Parallel()
	e := newBridgeEnv(t)
	start := e.clk.Now()

	e.wantRun(t.Context(), t, domain.StepSubmitted)
	if waited := e.clk.Now().Sub(start); waited != app.DefaultPoll().For || e.privy.polls != 7 {
		t.Fatalf("waited %s over %d polls, want the %s budget polled every %s",
			waited, e.privy.polls, app.DefaultPoll().For, app.DefaultPoll().Every)
	}
	e.privy.script(multichain.ActionSucceeded)
	e.wantRun(t.Context(), t, domain.StepSucceeded)

	e.wantOneTransfer(t, 1)
}

func TestBridge_aFailedOrRejectedActionFailsTheStep(t *testing.T) {
	t.Parallel()
	for _, state := range []multichain.ActionState{multichain.ActionFailed, multichain.ActionRejected} {
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()
			e := newBridgeEnv(t)
			e.privy.script(state)

			e.wantRun(t.Context(), t, domain.StepFailed)
			e.wantRun(t.Context(), t, domain.StepFailed)

			if s := e.step(t); s.Error != "privy_"+string(state) || s.PrivyActionID != "privy-action-1" {
				t.Fatalf("step = %+v, want failed with privy_%s", s, state)
			}
			e.wantOneTransfer(t, 1)
		})
	}
}

func TestBridge_aRefusedTransferFailsTheStep(t *testing.T) {
	t.Parallel()
	refused := newBridgeEnv(t)
	refused.privy.transferErr = errs.New(errs.CodeInvalidInput, "privy.Transfer")
	refused.wantRun(t.Context(), t, domain.StepFailed)
	if s := refused.step(t); s.Error != string(errs.CodeInvalidInput) || s.PrivyActionID != "" {
		t.Fatalf("refused step = %+v, want failed invalid_input with no action", s)
	}
}

func TestBridge_aTransferOutageLeavesTheStepPending(t *testing.T) {
	t.Parallel()
	outage := newBridgeEnv(t)
	outage.privy.transferErr = errs.New(errs.CodePrivyUnavailable, "privy.Transfer")
	if _, err := outage.run(t.Context()); errs.CodeOf(err) != errs.CodePrivyUnavailable {
		t.Fatalf("Run err = %v, want privy_unavailable", err)
	}
	if s := outage.step(t); s.Status != domain.StepPending {
		t.Fatalf("step after the outage = %+v, want pending", s)
	}
	wantEvents(t, outage.events(t), outage.event(domain.StepPending, "", 0))
}

func TestBridge_anActionReadErrorReturnsTheError(t *testing.T) {
	t.Parallel()
	read := newBridgeEnv(t)
	read.privy.actionErr = errs.New(errs.CodePrivyUnavailable, "privy.Action")
	if _, err := read.run(t.Context()); errs.CodeOf(err) != errs.CodePrivyUnavailable {
		t.Fatalf("Run err = %v, want privy_unavailable", err)
	}
}

func TestBridge_aCancelledWaitLeavesTheStepSubmitted(t *testing.T) {
	t.Parallel()
	waiting := newBridgeEnv(t)
	waiting.journal = waiting.journalOn(waiting.clk)
	ctx, cancel := context.WithCancel(t.Context())
	waiting.privy.afterAction = cancel
	if _, err := waiting.run(ctx); errs.CodeOf(err) != errs.CodeUpstreamTimeout {
		t.Fatalf("Run err = %v, want upstream_timeout from the cancelled wait", err)
	}
	if s := waiting.step(t); s.Status != domain.StepSubmitted {
		t.Fatalf("step = %+v, want submitted for the resume", s)
	}
}

func TestBridge_aLegOnAnotherNetworkThanTheStoredStepIsRefused(t *testing.T) {
	t.Parallel()
	e := newBridgeEnv(t)
	e.begin(t, 10_000_000)
	e.leg.Ref.Network = chain.NetworkArbitrum

	if _, err := e.run(t.Context()); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Run err = %v, want invalid_input", err)
	}
	if len(e.privy.calls()) != 0 {
		t.Fatal("a refused leg called Privy")
	}
}
