package trading_test

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	busevents "github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type stepEnv struct {
	swapDB
	journal app.Journal
	ref     domain.StepRef
}

func newStepEnv(t *testing.T) stepEnv {
	t.Helper()
	d := newSwapDB(t)
	d.now = clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	row := d.created(d.ids.NewV7(), usdcMint)
	d.insert(t, row)
	clk := testkit.NewClock(d.now)
	return stepEnv{
		swapDB:  d,
		journal: app.Journal{UoW: db.New(d.pool, d.ids, clk), Reads: d.pool, Clock: clk, IDs: d.ids},
		ref: domain.StepRef{
			SwapID: ids.SwapIDFrom(row.ID), CabalID: ids.CabalIDFrom(row.CabalID),
			Step: domain.StepBridgeOut, Network: chain.NetworkBase,
		},
	}
}

func stepCtx(t *testing.T) context.Context {
	t.Helper()
	return observability.WithActor(t.Context(), "system:trade-engine")
}

func (e stepEnv) events(t *testing.T) []busevents.TradeStepChanged {
	t.Helper()
	rows, err := e.pool.Query(t.Context(),
		`SELECT payload FROM events WHERE type = 'trade.step_changed' AND aggregate_id = $1 ORDER BY id`,
		e.ref.SwapID.UUID())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []busevents.TradeStepChanged
	for rows.Next() {
		var raw []byte
		var ev busevents.TradeStepChanged
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &ev); err != nil {
			t.Fatal(err)
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (e stepEnv) step(t *testing.T) domain.TradeStep {
	t.Helper()
	s, ok, err := e.journal.Step(stepCtx(t), e.ref.SwapID, e.ref.Step)
	if err != nil || !ok {
		t.Fatalf("Step = %+v, %v, %v, want the stored row", s, ok, err)
	}
	return s
}

func (e stepEnv) begin(t *testing.T, amount uint64) domain.TradeStep {
	t.Helper()
	s, err := e.journal.Begin(stepCtx(t), e.ref, money.MicrosFromUint64(amount))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func wantMoved(t *testing.T, what string, moved bool, err error, want bool) {
	t.Helper()
	if err != nil || moved != want {
		t.Fatalf("%s = %v, %v, want %v", what, moved, err, want)
	}
}

func (e stepEnv) event(status domain.StepStatus, action string, fee uint64) busevents.TradeStepChanged {
	return busevents.TradeStepChanged{
		V: 1, SwapID: e.ref.SwapID.UUID(), CabalID: e.ref.CabalID.UUID(), Step: string(e.ref.Step),
		Network: e.ref.Network, Status: string(status), PrivyActionID: action,
		AmountMicros: money.MicrosFromUint64(10_000_000), FeeMicros: money.MicrosFromUint64(fee),
	}
}

func wantEvents(t *testing.T, got []busevents.TradeStepChanged, want ...busevents.TradeStepChanged) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestJournal_beginInsertsOnePendingRowAndAResumedBeginReturnsItWithoutAnEvent(t *testing.T) {
	t.Parallel()
	e := newStepEnv(t)

	first := e.begin(t, 10_000_000)
	again := e.begin(t, 99_000_000)

	if first.Status != domain.StepPending || first.Amount != money.MicrosFromUint64(10_000_000) ||
		first.Network != chain.NetworkBase || first.PrivyActionID != "" || again != first {
		t.Fatalf("Begin = %+v then %+v, want one pending 10 USDC base row returned both times", first, again)
	}
	wantEvents(t, e.events(t), e.event(domain.StepPending, "", 0))
}

func TestJournal_submittedMovesPendingOnceAndAResumeKeepsTheStoredActionID(t *testing.T) {
	t.Parallel()
	e := newStepEnv(t)
	e.begin(t, 10_000_000)

	moved, err := e.journal.Submitted(stepCtx(t), e.ref, "privy-action-1")
	wantMoved(t, "first Submitted", moved, err, true)
	moved, err = e.journal.Submitted(stepCtx(t), e.ref, "privy-action-2")
	wantMoved(t, "resumed Submitted", moved, err, false)
	resumed := e.begin(t, 10_000_000)

	if s := e.step(t); s.Status != domain.StepSubmitted || s.PrivyActionID != "privy-action-1" || resumed != s {
		t.Fatalf("step = %+v, resumed Begin = %+v, want submitted with privy-action-1 both times", s, resumed)
	}
	wantEvents(t, e.events(t),
		e.event(domain.StepPending, "", 0),
		e.event(domain.StepSubmitted, "privy-action-1", 0),
	)
}

func TestJournal_succeededCarriesTheFeeOnceAndAFinishedStepNeverMovesAgain(t *testing.T) {
	t.Parallel()
	e := newStepEnv(t)
	e.begin(t, 10_000_000)
	moved, err := e.journal.Submitted(stepCtx(t), e.ref, "privy-action-1")
	wantMoved(t, "Submitted", moved, err, true)

	moved, err = e.journal.Succeeded(stepCtx(t), e.ref, money.MicrosFromUint64(7_500))
	wantMoved(t, "Succeeded", moved, err, true)
	moved, err = e.journal.Succeeded(stepCtx(t), e.ref, money.MicrosFromUint64(7_500))
	wantMoved(t, "second Succeeded", moved, err, false)
	moved, err = e.journal.Failed(stepCtx(t), e.ref, "late failure")
	wantMoved(t, "Failed after success", moved, err, false)
	moved, err = e.journal.Submitted(stepCtx(t), e.ref, "privy-action-2")
	wantMoved(t, "Submitted after success", moved, err, false)

	if s := e.step(t); s.Status != domain.StepSucceeded || s.Error != "" || s.PrivyActionID != "privy-action-1" {
		t.Fatalf("step = %+v, want succeeded with privy-action-1 and no error", s)
	}
	wantEvents(t, e.events(t),
		e.event(domain.StepPending, "", 0),
		e.event(domain.StepSubmitted, "privy-action-1", 0),
		e.event(domain.StepSucceeded, "privy-action-1", 7_500),
	)
}

func TestJournal_failedStoresTheReasonFromPendingOrSubmitted(t *testing.T) {
	t.Parallel()
	for name, submit := range map[string]bool{"pending": false, "submitted": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newStepEnv(t)
			e.begin(t, 10_000_000)
			want := []busevents.TradeStepChanged{e.event(domain.StepPending, "", 0)}
			action := ""
			if submit {
				action = "privy-action-1"
				moved, err := e.journal.Submitted(stepCtx(t), e.ref, action)
				wantMoved(t, "Submitted", moved, err, true)
				want = append(want, e.event(domain.StepSubmitted, action, 0))
			}

			moved, err := e.journal.Failed(stepCtx(t), e.ref, "bridge reverted")
			wantMoved(t, "Failed", moved, err, true)
			moved, err = e.journal.Failed(stepCtx(t), e.ref, "bridge reverted again")
			wantMoved(t, "second Failed", moved, err, false)
			moved, err = e.journal.Succeeded(stepCtx(t), e.ref, money.Micros{})
			wantMoved(t, "Succeeded after failure", moved, err, false)

			s := e.step(t)
			if s.Status != domain.StepFailed || s.Error != "bridge reverted" || s.PrivyActionID != action {
				t.Fatalf("step = %+v, want failed with the first reason and action %q", s, action)
			}
			wantEvents(t, e.events(t), append(want, e.event(domain.StepFailed, action, 0))...)
		})
	}
}

func TestJournal_aFeeAboveTheAmountRollsTheSuccessBack(t *testing.T) {
	t.Parallel()
	e := newStepEnv(t)
	e.begin(t, 10_000_000)
	moved, err := e.journal.Submitted(stepCtx(t), e.ref, "privy-action-1")
	wantMoved(t, "Submitted", moved, err, true)

	moved, err = e.journal.Succeeded(stepCtx(t), e.ref, money.MicrosFromUint64(10_000_001))

	if errs.CodeOf(err) != errs.CodeInvalidInput || moved {
		t.Fatalf("Succeeded = %v, %v, want invalid_input", moved, err)
	}
	if s := e.step(t); s.Status != domain.StepSubmitted {
		t.Fatalf("step = %+v, want still submitted", s)
	}
	if n := len(e.events(t)); n != 2 {
		t.Fatalf("%d events, want pending and submitted only", n)
	}
}

func TestJournal_refusesEmptyInputsAndReportsAMissingStep(t *testing.T) {
	t.Parallel()
	e := newStepEnv(t)

	if _, err := e.journal.Begin(stepCtx(t), e.ref, money.Micros{}); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Begin(0) err = %v, want invalid_input", err)
	}
	if _, ok, err := e.journal.Step(stepCtx(t), e.ref.SwapID, e.ref.Step); ok || err != nil {
		t.Fatalf("Step before Begin = %v, %v, want missing", ok, err)
	}
	moved, err := e.journal.Submitted(stepCtx(t), e.ref, "privy-action-1")
	wantMoved(t, "Submitted before Begin", moved, err, false)
	e.begin(t, 10_000_000)
	if _, err := e.journal.Submitted(stepCtx(t), e.ref, ""); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Submitted(\"\") err = %v, want invalid_input", err)
	}
	if _, err := e.journal.Failed(stepCtx(t), e.ref, ""); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Failed(\"\") err = %v, want invalid_input", err)
	}
	if s := e.step(t); s.Status != domain.StepPending {
		t.Fatalf("step = %+v, want still pending", s)
	}
	if n := len(e.events(t)); n != 1 {
		t.Fatalf("%d events, want the pending one only", n)
	}
}

func TestJournal_beginForAnUnknownSwapFailsWithoutAnEvent(t *testing.T) {
	t.Parallel()
	e := newStepEnv(t)
	e.ref.SwapID = ids.SwapIDFrom(e.ids.NewV7())

	if _, err := e.journal.Begin(stepCtx(t), e.ref, money.MicrosFromUint64(10_000_000)); err == nil {
		t.Fatal("Begin for a swap with no row succeeded, want the foreign key error")
	}
	if n := len(e.events(t)); n != 0 {
		t.Fatalf("%d events, want none", n)
	}
}

func TestJournal_anAmountPastUint64IsAnErrorAndRollsTheMoveBack(t *testing.T) {
	t.Parallel()
	e := newStepEnv(t)
	e.begin(t, 10_000_000)
	if _, err := e.pool.Exec(t.Context(), `UPDATE trade_steps SET amount_micros = 18446744073709551616
		WHERE trade_id = $1`, e.ref.SwapID.UUID()); err != nil {
		t.Fatal(err)
	}

	if _, _, err := e.journal.Step(stepCtx(t), e.ref.SwapID, e.ref.Step); err == nil {
		t.Fatal("Step read an amount past uint64, want an error")
	}
	if _, err := e.journal.Begin(stepCtx(t), e.ref, money.MicrosFromUint64(10_000_000)); err == nil {
		t.Fatal("a resumed Begin read an amount past uint64, want an error")
	}
	moved, err := e.journal.Submitted(stepCtx(t), e.ref, "privy-action-1")
	if err == nil || moved {
		t.Fatalf("Submitted = %v, %v, want an error", moved, err)
	}
	var status string
	if err := e.pool.QueryRow(t.Context(), `SELECT status FROM trade_steps WHERE trade_id = $1`,
		e.ref.SwapID.UUID()).Scan(&status); err != nil || status != string(domain.StepPending) {
		t.Fatalf("status = %s, %v, want pending after the rollback", status, err)
	}
	if n := len(e.events(t)); n != 1 {
		t.Fatalf("%d events, want the pending one only", n)
	}
}

func TestJournal_stepReturnsTheReadError(t *testing.T) {
	t.Parallel()
	e := newStepEnv(t)
	ctx, cancel := context.WithCancel(stepCtx(t))
	cancel()

	if _, ok, err := e.journal.Step(ctx, e.ref.SwapID, e.ref.Step); err == nil || ok ||
		errs.CodeOf(err) == errs.CodeNotFound {
		t.Fatalf("Step on a cancelled context = %v, %v, want the read error", ok, err)
	}
}

func TestJournal_beginRefusesAStoredStepOnAnotherNetwork(t *testing.T) {
	t.Parallel()
	e := newStepEnv(t)
	e.begin(t, 10_000_000)
	e.ref.Network = chain.NetworkArbitrum

	if _, err := e.journal.Begin(
		stepCtx(t),
		e.ref,
		money.MicrosFromUint64(10_000_000),
	); errs.CodeOf(
		err,
	) != errs.CodeInvalidInput {
		t.Fatalf("Begin on arbitrum err = %v, want invalid_input", err)
	}
	if s := e.step(t); s.Network != chain.NetworkBase || s.Status != domain.StepPending {
		t.Fatalf("step = %+v, want the pending base row untouched", s)
	}
}

type stepLog struct {
	Msg          string `json:"msg"`
	SwapID       string `json:"swap_id"`
	CabalID      string `json:"cabal_id"`
	Step         string `json:"step"`
	Network      string `json:"network"`
	BeforeStatus string `json:"before_status"`
	Status       string `json:"status"`
	AmountMicros string `json:"amount_micros"`
	FeeMicros    string `json:"fee_micros"`
}

func stepLogs(t *testing.T, logs *testkit.Logs) []stepLog {
	t.Helper()
	var out []stepLog
	for line := range bytes.Lines(logs.Bytes()) {
		var l stepLog
		if err := json.Unmarshal(line, &l); err != nil {
			t.Fatal(err)
		}
		if l.Msg == observability.TradingStepChanged.Name {
			out = append(out, l)
		}
	}
	return out
}

func TestJournal_logsEachCommittedTransitionOnceAfterTheCommit(t *testing.T) {
	t.Parallel()
	e := newStepEnv(t)
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(stepCtx(t), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))

	if _, err := e.journal.Begin(ctx, e.ref, money.MicrosFromUint64(10_000_000)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.journal.Begin(ctx, e.ref, money.MicrosFromUint64(10_000_000)); err != nil {
		t.Fatal(err)
	}
	moved, err := e.journal.Submitted(ctx, e.ref, "privy-action-1")
	wantMoved(t, "Submitted", moved, err, true)
	if _, err := e.journal.Succeeded(ctx, e.ref, money.MicrosFromUint64(10_000_001)); err == nil {
		t.Fatal("a fee above the amount succeeded")
	}
	moved, err = e.journal.Succeeded(ctx, e.ref, money.MicrosFromUint64(7_500))
	wantMoved(t, "Succeeded", moved, err, true)
	moved, err = e.journal.Succeeded(ctx, e.ref, money.MicrosFromUint64(7_500))
	wantMoved(t, "second Succeeded", moved, err, false)

	line := func(before, status domain.StepStatus, fee string) stepLog {
		return stepLog{
			Msg: observability.TradingStepChanged.Name, SwapID: e.ref.SwapID.String(), CabalID: e.ref.CabalID.String(),
			Step: "bridge_out", Network: "base", BeforeStatus: string(before), Status: string(status),
			AmountMicros: "10000000", FeeMicros: fee,
		}
	}
	want := []stepLog{
		line("", domain.StepPending, "0"),
		line(domain.StepPending, domain.StepSubmitted, "0"),
		line(domain.StepSubmitted, domain.StepSucceeded, "7500"),
	}
	if got := stepLogs(t, logs); !slices.Equal(got, want) {
		t.Fatalf("step logs = %+v, want %+v", got, want)
	}
}
