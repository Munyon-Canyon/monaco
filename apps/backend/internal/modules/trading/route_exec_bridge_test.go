package trading_test

import (
	"bytes"
	"context"
	"log/slog"
	"slices"
	"strings"
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
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const evmTreasury = "0x00000000000000000000000000000000000000aa"

type bridgeEnv struct {
	stepEnv
	clk    *testkit.Clock
	privy  *privyFake
	topUps *topUpFake
	leg    app.BridgeLeg
	unsure *[]multichain.ActionID
	hook   *error
}

func newBridgeEnv(t *testing.T) bridgeEnv {
	t.Helper()
	e := bridgeEnv{stepEnv: newStepEnv(t), clk: testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))}
	e.journal = e.journalOn(steppingClock{e.clk})
	e.privy = newPrivyFake()
	e.topUps = &topUpFake{privy: e.privy}
	e.unsure = &[]multichain.ActionID{}
	e.hook = new(error)
	e.leg = app.BridgeLeg{
		Ref: e.ref, WalletID: treasuryWallet, Source: solanaTreasury, Fee: money.MicrosFromUint64(7_500),
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
	unknown := func(_ context.Context, _ app.BridgeLeg, id multichain.ActionID) error {
		*e.unsure = append(*e.unsure, id)
		return *e.hook
	}
	return app.Bridge{Journal: e.journal, Actions: e.privy, TopUp: e.topUps, Poll: app.DefaultPoll(), Unknown: unknown}.
		Run(actorContext(ctx), e.leg)
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
		e.event(domain.StepSubmitted, "privy-action-1", 7_500),
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
		e.event(domain.StepSubmitted, "privy-action-1", 7_500),
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

func TestBridge_aRejectedOrCleanlyFailedActionFailsTheStep(t *testing.T) {
	t.Parallel()
	refusedStep := []multichain.ActionStep{{Network: chain.NetworkSolana, State: multichain.StepFailed}}
	for name, c := range map[string]struct {
		state multichain.ActionState
		steps []multichain.ActionStep
	}{
		"rejected":                      {state: multichain.ActionRejected},
		"failed with no steps":          {state: multichain.ActionFailed},
		"failed with a refused step":    {state: multichain.ActionFailed, steps: refusedStep},
		"failed with only refused ones": {state: multichain.ActionFailed, steps: slices.Repeat(refusedStep, 2)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newBridgeEnv(t)
			e.privy.script(c.state)
			e.privy.failedSteps = c.steps

			e.wantRun(t.Context(), t, domain.StepFailed)
			e.wantRun(t.Context(), t, domain.StepFailed)

			if s := e.step(t); s.Error != "privy_"+string(c.state) || s.PrivyActionID != "privy-action-1" {
				t.Fatalf("step = %+v, want failed with privy_%s", s, c.state)
			}
			if len(*e.unsure) != 0 {
				t.Fatalf("unknown outcome reported %v for a failure with nothing broadcast", *e.unsure)
			}
			e.wantOneTransfer(t, 1)
		})
	}
}

func TestBridge_aFailedActionThatBroadcastIsAnUnknownOutcomeNotAReturn(t *testing.T) {
	t.Parallel()
	for name, steps := range map[string][]multichain.ActionStep{
		"confirmed on chain": {{Network: chain.NetworkSolana, TxHash: "solana-hash", State: multichain.StepConfirmed}},
		"reverted with hash": {{Network: chain.NetworkBase, TxHash: "0xhash", State: multichain.StepFailed}},
		"still pending":      {{Network: chain.NetworkSolana, State: multichain.StepPending}},
		"bridge leg":         {{State: multichain.StepFailed}},
		"after a refused one": {
			{Network: chain.NetworkSolana, State: multichain.StepFailed},
			{Network: chain.NetworkSolana, TxHash: "solana-hash", State: multichain.StepConfirmed},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newBridgeEnv(t)
			e.privy.script(multichain.ActionFailed)
			e.privy.failedSteps = steps
			var logs bytes.Buffer
			ctx := observability.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(&logs, nil)))

			e.wantRun(ctx, t, domain.StepSubmitted)

			if s := e.step(
				t,
			); s.Status != domain.StepSubmitted || s.Error != "" ||
				s.PrivyActionID != "privy-action-1" {
				t.Fatalf("step = %+v, want submitted with no error so nothing is booked back", s)
			}
			e.wantNoFailedEvent(t)
			if !slices.Equal(*e.unsure, []multichain.ActionID{"privy-action-1"}) {
				t.Fatalf("unknown outcome reported %v, want the action once", *e.unsure)
			}
			e.wantAlert(t, logs.String())
			e.wantOneTransfer(t, 1)
		})
	}
}

func (e bridgeEnv) wantNoFailedEvent(t *testing.T) {
	t.Helper()
	for _, ev := range e.events(t) {
		if ev.Status == string(domain.StepFailed) {
			t.Fatalf("events = %+v, want no failed step event", e.events(t))
		}
	}
}

func (bridgeEnv) wantAlert(t *testing.T, logs string) {
	t.Helper()
	if !strings.Contains(logs, "trading.bridge.outcome_unknown") || !strings.Contains(logs, `"level":"ERROR"`) ||
		!strings.Contains(logs, `"action_id":"privy-action-1"`) {
		t.Fatalf("logs = %s, want an alert naming the action", logs)
	}
}

func TestBridge_aRetryAfterAnUnknownOutcomeNeverSendsASecondTransfer(t *testing.T) {
	t.Parallel()
	e := newBridgeEnv(t)
	e.privy.script(multichain.ActionFailed, multichain.ActionFailed)
	e.privy.failedSteps = []multichain.ActionStep{
		{Network: chain.NetworkSolana, TxHash: "h", State: multichain.StepConfirmed},
	}

	e.wantRun(t.Context(), t, domain.StepSubmitted)
	e.wantRun(t.Context(), t, domain.StepSubmitted)

	e.wantOneTransfer(t, 1)
	if len(*e.unsure) != 2 {
		t.Fatalf("unknown outcome reported %v, want it again on the second look", *e.unsure)
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

func TestBridge_aSuccessResponseThatFailsToDecodeIsAnUnknownOutcomeNotAReturn(t *testing.T) {
	t.Parallel()
	e := newBridgeEnv(t)
	e.privy.transferErr = errs.New(errs.CodeDecodeFailed, "privy.Transfer")
	e.privy.script(multichain.ActionSucceeded)
	var logs bytes.Buffer
	ctx := observability.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(&logs, nil)))

	e.wantRun(ctx, t, domain.StepPending)

	if s := e.step(t); s.Status != domain.StepPending || s.Error != "" || s.PrivyActionID != "" {
		t.Fatalf("step = %+v, want pending with no error so nothing is booked back", s)
	}
	e.wantNoFailedEvent(t)
	if !slices.Equal(*e.unsure, []multichain.ActionID{""}) {
		t.Fatalf("unknown outcome reported %v, want it once with no action id", *e.unsure)
	}
	if !strings.Contains(logs.String(), "trading.bridge.outcome_unknown") ||
		!strings.Contains(logs.String(), `"level":"ERROR"`) {
		t.Fatalf("logs = %s, want an outcome_unknown alert", logs.String())
	}

	e.wantRun(t.Context(), t, domain.StepSucceeded)
	e.wantOneTransfer(t, 1)
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

func TestBridge_aPriorActionThatBroadcastThenFailedStopsTheLegAndSurfacesAParkFailure(t *testing.T) {
	t.Parallel()
	e := newBridgeEnv(t)
	e.begin(t, 10_000_000)
	if _, err := e.pool.Exec(t.Context(),
		`UPDATE trade_steps SET prior_privy_action_ids = ARRAY['privy-action-9'] WHERE trade_id = $1`,
		e.ref.SwapID.UUID()); err != nil {
		t.Fatal(err)
	}
	e.privy.script(multichain.ActionFailed)
	e.privy.failedSteps = []multichain.ActionStep{
		{Network: chain.NetworkSolana, TxHash: "h", State: multichain.StepConfirmed},
	}
	*e.hook = errs.New(errs.CodeInternal, "test.park")

	if _, err := e.run(t.Context()); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Run err = %v, want the park failure", err)
	}

	if len(e.privy.calls()) != 0 || !slices.Equal(*e.unsure, []multichain.ActionID{"privy-action-9"}) {
		t.Fatalf("transfers %v, unknown %v, want no send and the prior action reported", e.privy.calls(), *e.unsure)
	}
}

func TestBridge_theSubmittedEventCarriesTheQuotedFeeCappedAtTheAmountSent(t *testing.T) {
	t.Parallel()
	e := newBridgeEnv(t)
	e.leg.Fee = money.MicrosFromUint64(12_000_000)
	e.leg.Observe = func(context.Context, multichain.ActionStatus) (money.Micros, domain.Arrival, error) {
		return money.MicrosFromUint64(7_500), domain.Arrival{}, nil
	}
	e.privy.script(multichain.ActionSucceeded)

	e.wantRun(t.Context(), t, domain.StepSucceeded)

	got := e.events(t)
	if len(got) < 2 || got[1].Status != string(domain.StepSubmitted) ||
		got[1].FeeMicros != money.MicrosFromUint64(10_000_000) {
		t.Fatalf("events = %+v, want a submitted event carrying a fee capped at the 10 USDC sent", got)
	}
}
