package governance_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chaos"
)

type delivery struct {
	*chaos.Msg
	seen    uint64
	outcome bus.Outcome
	delay   time.Duration
}

func (m *delivery) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{NumDelivered: m.seen}, nil
}

func (m *delivery) Ack() error                      { m.outcome = bus.OutcomeAck; return nil }
func (m *delivery) DoubleAck(context.Context) error { return m.Ack() }
func (m *delivery) Nak() error                      { m.outcome = bus.OutcomeNak; return nil }
func (m *delivery) Term() error                     { m.outcome = bus.OutcomeTerm; return nil }
func (m *delivery) TermWithReason(string) error     { return m.Term() }

func (m *delivery) NakWithDelay(d time.Duration) error {
	m.outcome, m.delay = bus.OutcomeNak, d
	return nil
}

type outcomeDB struct {
	voteDB
	conn *bus.Conn
	reg  *bus.Registry
}

func newOutcomeDB(t *testing.T) outcomeDB {
	t.Helper()
	d := newVoteDB(t)
	conn := testkit.NATS(t).Conn
	consumers := governance.New(module.Deps{Pool: d.pool, IDs: d.ids, Clock: d.clk, Bus: conn}).Consumers()
	reg, err := bus.NewRegistry(conn, d.uow, d.clk, consumers)
	if err != nil {
		t.Fatal(err)
	}
	return outcomeDB{voteDB: d, conn: conn, reg: reg}
}

func (d outcomeDB) proposal(t *testing.T, status string) uuid.UUID {
	t.Helper()
	p, _ := d.open(t, 1)
	if _, err := d.pool.Exec(t.Context(), `UPDATE proposals SET status = $2 WHERE id = $1`, p.ID, status); err != nil {
		t.Fatal(err)
	}
	return p.ID
}

func (d outcomeDB) publish(t *testing.T, ev events.Event) *delivery {
	t.Helper()
	ctx := observability.WithActor(t.Context(), "system:trading")
	err := d.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error { return tx.Events.Append(ctx, ev) })
	if err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	var payload []byte
	err = d.pool.QueryRow(t.Context(),
		`SELECT id, payload FROM events WHERE type = $1 ORDER BY id DESC LIMIT 1`, string(ev.Type())).
		Scan(&id, &payload)
	if err != nil {
		t.Fatal(err)
	}
	return &delivery{Msg: chaos.NewMsg(d.conn, ev.Type(), ids.EventIDFrom(id), payload)}
}

func (d outcomeDB) deliver(t *testing.T, m *delivery) bus.Outcome {
	t.Helper()
	m.seen++
	m.outcome, m.delay = "", 0
	d.reg.Dispatch(observability.WithActor(t.Context(), "system:worker"), "governance", m)
	return m.outcome
}

func (d outcomeDB) confirmed(proposal uuid.UUID, kind string) events.TradeConfirmed {
	return events.TradeConfirmed{
		V: 1, SwapID: d.ids.NewV7(), CabalID: d.ids.NewV7(), Source: events.TradeSource{Kind: kind, ID: proposal},
		SourceBatchSize: 1, Action: "buy", Symbol: "AAPLx", InAmount: 25_000_000, OutAmount: 105_000_000,
		ConfirmedAt: d.now,
	}
}

func (d outcomeDB) blocked(proposal uuid.UUID, kind string, code errs.Code) events.TradeBlocked {
	return events.TradeBlocked{
		V: 1, CabalID: d.ids.NewV7(), Source: events.TradeSource{Kind: kind, ID: proposal}, SourceBatchSize: 1,
		Action: "buy", Symbol: "AAPLx", Code: code, Have: 10_000_000, Need: 25_000_000,
	}
}

func (d outcomeDB) outcomes(t *testing.T, proposal uuid.UUID) int {
	t.Helper()
	return len(d.payloads(t, proposal, events.TypeProposalExecuted)) +
		len(d.payloads(t, proposal, events.TypeProposalExecutionBlocked))
}

func (d outcomeDB) wantRow(t *testing.T, proposal uuid.UUID, status, reason string) {
	t.Helper()
	if r := d.row(t, proposal); r.status.String != status || r.statusReason.String != reason {
		t.Errorf("proposal row = %s %q, want %s %q", r.status.String, r.statusReason.String, status, reason)
	}
}

func TestTradeOutcome_Confirmed_MarksExecuted(t *testing.T) {
	t.Parallel()
	d := newOutcomeDB(t)
	p := d.proposal(t, "passed")
	ev := d.confirmed(p, "proposal")
	if got := d.deliver(t, d.publish(t, ev)); got != bus.OutcomeAck {
		t.Fatalf("trade.confirmed for a passed proposal = %s, want ack", got)
	}
	d.wantRow(t, p, "executed", "")
	raw := d.payloads(t, p, events.TypeProposalExecuted)
	var got events.ProposalExecuted
	if len(raw) != 1 || json.Unmarshal(raw[0], &got) != nil {
		t.Fatalf("proposal.executed payloads = %s, want one", raw)
	}
	want := events.ProposalExecuted{V: 1, ProposalID: p, CabalID: ev.CabalID, SwapID: ev.SwapID}
	if got != want {
		t.Fatalf("proposal.executed = %+v, want %+v", got, want)
	}
}

func TestTradeOutcome_Blocked_MarksExecutionBlocked(t *testing.T) {
	t.Parallel()
	d := newOutcomeDB(t)
	p := d.proposal(t, "passed")
	ev := d.blocked(p, "proposal", errs.CodeInsufficientFunds)
	if got := d.deliver(t, d.publish(t, ev)); got != bus.OutcomeAck {
		t.Fatalf("trade.blocked for a passed proposal = %s, want ack", got)
	}
	d.wantRow(t, p, "execution_blocked", string(errs.CodeInsufficientFunds))
	raw := d.payloads(t, p, events.TypeProposalExecutionBlocked)
	var got events.ProposalExecutionBlocked
	if len(raw) != 1 || json.Unmarshal(raw[0], &got) != nil {
		t.Fatalf("proposal.execution_blocked payloads = %s, want one", raw)
	}
	want := events.ProposalExecutionBlocked{V: 1, ProposalID: p, CabalID: ev.CabalID, Code: errs.CodeInsufficientFunds}
	if got != want {
		t.Fatalf("proposal.execution_blocked = %+v, want %+v", got, want)
	}
}

func (d outcomeDB) failed(proposal uuid.UUID, kind string) events.TradeFailed {
	return events.TradeFailed{
		V: 1, SwapID: d.ids.NewV7(), CabalID: d.ids.NewV7(), Source: events.TradeSource{Kind: kind, ID: proposal},
		SourceBatchSize: 1, Action: "sell", Symbol: "AAPLx", InAmount: 25_000_000, FailureCode: "jupiter_failed",
	}
}

func TestTradeOutcome_Failed_MarksExecutionBlockedWithSwapFailed(t *testing.T) {
	t.Parallel()
	d := newOutcomeDB(t)
	p := d.proposal(t, "passed")
	ev := d.failed(p, "proposal")
	if got := d.deliver(t, d.publish(t, ev)); got != bus.OutcomeAck {
		t.Fatalf("trade.failed for a passed proposal = %s, want ack", got)
	}
	d.wantRow(t, p, "execution_blocked", string(errs.CodeSwapFailed))
	raw := d.payloads(t, p, events.TypeProposalExecutionBlocked)
	var got events.ProposalExecutionBlocked
	if len(raw) != 1 || json.Unmarshal(raw[0], &got) != nil {
		t.Fatalf("proposal.execution_blocked payloads = %s, want one", raw)
	}
	want := events.ProposalExecutionBlocked{V: 1, ProposalID: p, CabalID: ev.CabalID, Code: errs.CodeSwapFailed}
	if got != want {
		t.Fatalf("proposal.execution_blocked = %+v, want %+v", got, want)
	}
}

func (d outcomeDB) retried(proposal uuid.UUID, kind string) events.TradeRetryRequested {
	return events.TradeRetryRequested{
		V: 1, SwapID: d.ids.NewV7(), CabalID: d.ids.NewV7(), Source: events.TradeSource{Kind: kind, ID: proposal},
		Action: "sell", Symbol: "AAPLx", InAmount: 25_000_000, RequestedBy: d.ids.NewV7(),
	}
}

func (d outcomeDB) block(t *testing.T, proposal uuid.UUID, reason string) {
	t.Helper()
	_, err := d.pool.Exec(t.Context(),
		`UPDATE proposals SET status = 'execution_blocked', status_reason = $2 WHERE id = $1`, proposal, reason)
	if err != nil {
		t.Fatal(err)
	}
}

func TestTradeOutcome_FailedThenRetriedThenConfirmed_ExecutesTheProposal(t *testing.T) {
	t.Parallel()
	d := newOutcomeDB(t)
	p := d.proposal(t, "passed")
	steps := []struct {
		ev         events.Event
		status     string
		wantReason string
	}{
		{d.failed(p, "proposal"), "execution_blocked", string(errs.CodeSwapFailed)},
		{d.retried(p, "proposal"), "passed", ""},
		{d.confirmed(p, "proposal"), "executed", ""},
	}
	for _, step := range steps {
		if got := d.deliver(t, d.publish(t, step.ev)); got != bus.OutcomeAck {
			t.Fatalf("%s = %s, want ack", step.ev.Type(), got)
		}
		d.wantRow(t, p, step.status, step.wantReason)
	}
	raw := d.payloads(t, p, events.TypeProposalReopened)
	var got events.ProposalReopened
	if len(raw) != 1 || json.Unmarshal(raw[0], &got) != nil || got.ProposalID != p {
		t.Fatalf("proposal.reopened payloads = %s, want one for %s", raw, p)
	}
}

func TestTradeOutcome_Retried_ReopensOnlyAProposalBlockedBySwapFailure(t *testing.T) {
	t.Parallel()
	d := newOutcomeDB(t)
	blockedOther := d.proposal(t, "passed")
	d.block(t, blockedOther, string(errs.CodeNoRoute))
	for name, tc := range map[string]struct {
		proposal uuid.UUID
		kind     string
		status   string
		reason   string
	}{
		"passed":           {d.proposal(t, "passed"), "proposal", "passed", ""},
		"executed":         {d.proposal(t, "executed"), "proposal", "executed", ""},
		"blocked by other": {blockedOther, "proposal", "execution_blocked", string(errs.CodeNoRoute)},
		"cashout source":   {d.proposal(t, "passed"), "cashout", "passed", ""},
	} {
		if got := d.deliver(t, d.publish(t, d.retried(tc.proposal, tc.kind))); got != bus.OutcomeAck {
			t.Errorf("%s: trade.retry_requested = %s, want ack", name, got)
		}
		d.wantRow(t, tc.proposal, tc.status, tc.reason)
		if n := len(d.payloads(t, tc.proposal, events.TypeProposalReopened)); n != 0 {
			t.Errorf("%s: %d proposal.reopened events, want 0", name, n)
		}
	}
}

func TestTradeOutcome_BeforeTheReopenLands_SettlesFromSwapFailure(t *testing.T) {
	t.Parallel()
	d := newOutcomeDB(t)
	confirmed := d.proposal(t, "passed")
	refused := d.proposal(t, "passed")
	again := d.proposal(t, "passed")
	for _, p := range []uuid.UUID{confirmed, refused, again} {
		d.block(t, p, string(errs.CodeSwapFailed))
	}
	for _, step := range []struct {
		proposal uuid.UUID
		ev       events.Event
		status   string
		reason   string
	}{
		{confirmed, d.confirmed(confirmed, "proposal"), "executed", ""},
		{refused, d.blocked(refused, "proposal", errs.CodeNoRoute), "execution_blocked", string(errs.CodeNoRoute)},
		{again, d.failed(again, "proposal"), "execution_blocked", string(errs.CodeSwapFailed)},
	} {
		if got := d.deliver(t, d.publish(t, step.ev)); got != bus.OutcomeAck {
			t.Errorf("%s on a swap_failed proposal = %s, want ack", step.ev.Type(), got)
		}
		d.wantRow(t, step.proposal, step.status, step.reason)
	}
	if n := d.outcomes(t, again); n != 0 {
		t.Errorf("a second trade.failed left %d outcome events, want 0", n)
	}
	if got := d.deliver(t, d.publish(t, d.retried(refused, "proposal"))); got != bus.OutcomeAck {
		t.Fatalf("late trade.retry_requested = %s, want ack", got)
	}
	d.wantRow(t, refused, "execution_blocked", string(errs.CodeNoRoute))
}

func TestTradeOutcome_Retried_DuplicateReopensOnce(t *testing.T) {
	t.Parallel()
	d := newOutcomeDB(t)
	p := d.proposal(t, "passed")
	d.block(t, p, string(errs.CodeSwapFailed))
	m := d.publish(t, d.retried(p, "proposal"))
	for range 2 {
		if got := d.deliver(t, m); got != bus.OutcomeAck {
			t.Fatalf("delivery = %s, want ack", got)
		}
	}
	if n := len(d.payloads(t, p, events.TypeProposalReopened)); n != 1 {
		t.Fatalf("%d proposal.reopened events after two deliveries, want 1", n)
	}
}

func TestTradeOutcome_Duplicate_OneEvent(t *testing.T) {
	t.Parallel()
	d := newOutcomeDB(t)
	for name, outcome := range map[string]func(uuid.UUID) events.Event{
		"confirmed": func(p uuid.UUID) events.Event { return d.confirmed(p, "proposal") },
		"blocked":   func(p uuid.UUID) events.Event { return d.blocked(p, "proposal", errs.CodeSlippageExceeded) },
		"failed":    func(p uuid.UUID) events.Event { return d.failed(p, "proposal") },
	} {
		p := d.proposal(t, "passed")
		m := d.publish(t, outcome(p))
		again := d.publish(t, outcome(p))
		for _, delivered := range []*delivery{m, m, again, again} {
			if got := d.deliver(t, delivered); got != bus.OutcomeAck {
				t.Errorf("%s: delivery = %s, want ack", name, got)
			}
		}
		if n := d.outcomes(t, p); n != 1 {
			t.Errorf("%s: two deliveries of two events left %d outcome events, want 1", name, n)
		}
	}
}

func TestTradeOutcome_Voided_AcksSilently(t *testing.T) {
	t.Parallel()
	d := newOutcomeDB(t)
	p := d.proposal(t, "voided")
	for _, ev := range []events.Event{
		d.confirmed(p, "proposal"), d.blocked(p, "proposal", errs.CodeNoRoute), d.failed(p, "proposal"),
	} {
		if got := d.deliver(t, d.publish(t, ev)); got != bus.OutcomeAck {
			t.Errorf("%s for a voided proposal = %s, want ack", ev.Type(), got)
		}
	}
	d.wantRow(t, p, "voided", "")
	if n := d.outcomes(t, p); n != 0 {
		t.Errorf("voided proposal has %d outcome events, want 0", n)
	}
}

func TestTradeOutcome_CashoutSource_Ignored(t *testing.T) {
	t.Parallel()
	d := newOutcomeDB(t)
	p := d.proposal(t, "passed")
	for _, kind := range []string{"cashout", "agent_intent"} {
		for _, ev := range []events.Event{
			d.confirmed(p, kind), d.blocked(p, kind, errs.CodeNoRoute), d.failed(p, kind),
		} {
			if got := d.deliver(t, d.publish(t, ev)); got != bus.OutcomeAck {
				t.Errorf("%s from a %s = %s, want ack", ev.Type(), kind, got)
			}
		}
	}
	d.wantRow(t, p, "passed", "")
	if n := d.outcomes(t, p); n != 0 {
		t.Errorf("a proposal whose id another source reused has %d outcome events, want 0", n)
	}
}

func TestTradeOutcome_StillOpen_Naks(t *testing.T) {
	t.Parallel()
	d := newOutcomeDB(t)
	p := d.proposal(t, "open")
	m := d.publish(t, d.confirmed(p, "proposal"))
	if got := d.deliver(t, m); got != bus.OutcomeNak || m.delay != bus.NakSchedule()[0] {
		t.Fatalf("trade.confirmed for an open proposal = %s after %s, want nak after %s",
			got, m.delay, bus.NakSchedule()[0])
	}
	d.wantRow(t, p, "open", "")
	if _, err := d.pool.Exec(t.Context(), `UPDATE proposals SET status = 'passed' WHERE id = $1`, p); err != nil {
		t.Fatal(err)
	}
	if got := d.deliver(t, m); got != bus.OutcomeAck {
		t.Fatalf("redelivery once the proposal passed = %s, want ack", got)
	}
	d.wantRow(t, p, "executed", "")
}

func TestTradeOutcome_TermsWhatItCannotApply(t *testing.T) {
	t.Parallel()
	d := newOutcomeDB(t)
	executed := d.proposal(t, "executed")
	failing := d.proposal(t, "passed")
	_, err := d.pool.Exec(t.Context(),
		`ALTER TABLE proposals ADD CONSTRAINT never_blocked CHECK (status <> 'execution_blocked') NOT VALID`)
	if err != nil {
		t.Fatal(err)
	}
	for name, ev := range map[string]events.Event{
		"an unknown proposal":                    d.confirmed(d.ids.NewV7(), "proposal"),
		"a blocked trade on executed":            d.blocked(executed, "proposal", errs.CodeNoRoute),
		"a failed swap on executed":              d.failed(executed, "proposal"),
		"an update the database refuses":         d.blocked(failing, "proposal", errs.CodeNoRoute),
		"a failed swap the database refuses":     d.failed(failing, "proposal"),
		"a confirmed trade on a closed proposal": d.confirmed(d.proposal(t, "expired"), "proposal"),
	} {
		if got := d.deliver(t, d.publish(t, ev)); got != bus.OutcomeTerm {
			t.Errorf("%s = %s, want term", name, got)
		}
	}
	d.wantRow(t, executed, "executed", "")
	d.wantRow(t, failing, "passed", "")
}

func TestTradeOutcome_TermsWhenAStoreWriteFails(t *testing.T) {
	t.Parallel()
	d := newOutcomeDB(t)
	reopen := d.proposal(t, "passed")
	appendFails := d.proposal(t, "passed")
	fallback := d.proposal(t, "passed")
	for _, p := range []uuid.UUID{reopen, appendFails, fallback} {
		d.block(t, p, string(errs.CodeSwapFailed))
	}
	for _, ddl := range []string{
		`ALTER TABLE proposals ADD CONSTRAINT never_executed CHECK (status <> 'executed') NOT VALID`,
		`ALTER TABLE events ADD CONSTRAINT never_reopened CHECK (type <> 'proposal.reopened') NOT VALID`,
	} {
		if _, err := d.pool.Exec(t.Context(), ddl); err != nil {
			t.Fatal(err)
		}
	}
	if got := d.deliver(t, d.publish(t, d.confirmed(fallback, "proposal"))); got != bus.OutcomeTerm {
		t.Errorf("trade.confirmed the database refuses = %s, want term", got)
	}
	d.wantRow(t, fallback, "execution_blocked", string(errs.CodeSwapFailed))
	if got := d.deliver(t, d.publish(t, d.retried(appendFails, "proposal"))); got != bus.OutcomeTerm {
		t.Errorf("trade.retry_requested whose event the database refuses = %s, want term", got)
	}
	d.wantRow(t, appendFails, "execution_blocked", string(errs.CodeSwapFailed))
}

func TestTradeOutcome_TermsWhenTheReopenIsRefused(t *testing.T) {
	t.Parallel()
	d := newOutcomeDB(t)
	p := d.proposal(t, "passed")
	d.block(t, p, string(errs.CodeSwapFailed))
	_, err := d.pool.Exec(t.Context(),
		`ALTER TABLE proposals ADD CONSTRAINT never_passed CHECK (status <> 'passed') NOT VALID`)
	if err != nil {
		t.Fatal(err)
	}
	if got := d.deliver(t, d.publish(t, d.retried(p, "proposal"))); got != bus.OutcomeTerm {
		t.Errorf("trade.retry_requested the database refuses = %s, want term", got)
	}
	d.wantRow(t, p, "execution_blocked", string(errs.CodeSwapFailed))
}
