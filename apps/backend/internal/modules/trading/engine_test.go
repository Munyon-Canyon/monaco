package trading_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	busevents "github.com/monaco/monaco/apps/backend/internal/events"
	governance "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chaos"
)

type engineMsg struct {
	*chaos.Msg
	id         ids.EventID
	payload    []byte
	delivered  uint64
	verdict    string
	inProgress atomic.Int32
	progressed chan struct{}
}

func (m *engineMsg) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{NumDelivered: m.delivered}, nil
}
func (m *engineMsg) Ack() error                       { m.verdict = "ack"; return nil }
func (m *engineMsg) DoubleAck(context.Context) error  { return m.Ack() }
func (m *engineMsg) Nak() error                       { m.verdict = "nak"; return nil }
func (m *engineMsg) NakWithDelay(time.Duration) error { return m.Nak() }
func (m *engineMsg) Term() error                      { m.verdict = "term"; return nil }
func (m *engineMsg) TermWithReason(string) error      { return m.Term() }

func (m *engineMsg) InProgress() error {
	m.inProgress.Add(1)
	if m.progressed != nil {
		m.progressed <- struct{}{}
	}
	return nil
}

type engineBus struct {
	*engineEnv
	conn *bus.Conn
	reg  *bus.Registry
}

func newEngineBus(t *testing.T, opts ...func(*engineEnv)) *engineBus {
	t.Helper()
	e := newEngineEnv(t)
	for _, opt := range opts {
		opt(e)
	}
	conn := testkit.NATS(t).Conn
	m := trading.New(module.Deps{
		Config: testkit.Config(), Clock: e.clk, IDs: e.ids, Pool: e.pool, UoW: e.uow, Bus: conn,
	}, trading.WithEnginePorts(e.ports()), trading.WithChain(e.venue, e.signer))
	reg, err := bus.NewRegistry(conn, e.uow, e.clk, m.Consumers())
	if err != nil {
		t.Fatal(err)
	}
	return &engineBus{engineEnv: e, conn: conn, reg: reg}
}

func (b *engineBus) message(t *testing.T, cmd app.ExecuteTrade, kind string) *engineMsg {
	t.Helper()
	d := b.delivery(t, cmd)
	var payload json.RawMessage
	if err := b.pool.QueryRow(t.Context(), `SELECT payload FROM events WHERE id = $1`, d.EventID.UUID()).
		Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if kind != "" {
		var p map[string]any
		_ = json.Unmarshal(payload, &p)
		p["kind"] = kind
		payload, _ = json.Marshal(p)
	}
	return &engineMsg{
		Msg: chaos.NewMsg(b.conn, busevents.TypeProposalPassed, d.EventID, payload),
		id:  d.EventID, payload: payload, delivered: 1,
	}
}

func (b *engineBus) dispatch(t *testing.T, msg *engineMsg) {
	t.Helper()
	b.reg.Dispatch(t.Context(), "trading", msg)
}

func (b *engineBus) redeliver(t *testing.T, msg *engineMsg) *engineMsg {
	t.Helper()
	again := &engineMsg{
		Msg: chaos.NewMsg(b.conn, busevents.TypeProposalPassed, msg.id, msg.payload),
		id:  msg.id, payload: msg.payload, delivered: msg.delivered + 1,
	}
	b.dispatch(t, again)
	return again
}

func (b *engineBus) statuses(t *testing.T, cmd app.ExecuteTrade) []string {
	t.Helper()
	swaps := b.swapsOf(t, cmd.ProposalID)
	out := make([]string, 0, len(swaps))
	for _, id := range swaps {
		status, _, _, _ := b.row(t, id)
		var code *string
		if err := b.pool.QueryRow(t.Context(), `SELECT failure_code FROM swaps WHERE id = $1`, id).
			Scan(&code); err != nil {
			t.Fatal(err)
		}
		if code != nil {
			status += ":" + *code
		}
		out = append(out, status)
	}
	return out
}

func (b *engineBus) recordedMsg(t *testing.T, msg *engineMsg) bool {
	t.Helper()
	return b.count(t, `SELECT count(*) FROM event_deliveries WHERE handler = $1 AND event_id::text = $2`,
		engineHandler, msg.id.String()) == 1
}

func TestTradeEngine_IgnoresAgentKinds(t *testing.T) {
	t.Parallel()
	b := newEngineBus(t)
	cmd := b.buy()
	msg := b.message(t, cmd, "agent_intent")

	b.dispatch(t, msg)
	if msg.verdict != "ack" || len(b.swapsOf(t, cmd.ProposalID)) != 0 || len(b.blocked(t, cmd.ProposalID)) != 0 {
		t.Fatalf("verdict %q, %d swaps, %d blocked; want an ack and nothing else", msg.verdict,
			len(b.swapsOf(t, cmd.ProposalID)), len(b.blocked(t, cmd.ProposalID)))
	}
}

func TestTradeEngine_RedeliveryAfterConfirm_NoSecondSwap(t *testing.T) {
	t.Parallel()
	b := newEngineBus(t)
	cmd := b.buy()
	msg := b.message(t, cmd, "")

	b.dispatch(t, msg)
	again := b.redeliver(t, msg)
	got := b.statuses(t, cmd)
	if msg.verdict != "ack" || again.verdict != "ack" || len(got) != 1 || got[0] != "confirmed" ||
		!b.recordedMsg(t, msg) || len(b.jup.Sent("req-1")) != 1 {
		t.Fatalf("verdicts %q, %q, swaps %v, %d executes; want two acks, one confirmed swap, one execute",
			msg.verdict, again.verdict, got, len(b.jup.Sent("req-1")))
	}
}

func TestTradeEngine_FailedRowAllowsNewClaim(t *testing.T) {
	t.Parallel()
	b := newEngineBus(t)
	cmd := b.buy()
	b.jup.SetExecute("req-1", jupiter.ExecuteResult{Status: jupiter.StatusFailed, ErrorCode: 6001})
	first := b.message(t, cmd, "")
	b.dispatch(t, first)
	retry := chainfake.Unsigned(chainfake.WalletAddress(treasuryWallet))
	retry[len(retry)-2] = 1
	b.jup.SetOrder(jupiterMint(usdcToken()), jupiterMint(aaplxToken()), jupiter.Order{
		RequestID: "req-2", Transaction: retry,
	})

	second := b.message(t, cmd, "")
	b.dispatch(t, second)
	got := b.statuses(t, cmd)
	if first.verdict != "ack" || second.verdict != "ack" || len(got) != 2 ||
		got[0] != "failed:jupiter_failed" || got[1] != "confirmed" {
		t.Fatalf("verdicts %q, %q, swaps %v; want the failed swap followed by a confirmed one",
			first.verdict, second.verdict, got)
	}
}

func TestTradeEngine_VoidedAfterClaim_SourceCancelled(t *testing.T) {
	t.Parallel()
	b := newEngineBus(t)
	b.proposals.set(governance.Status("voided"))
	cmd := b.buy()
	msg := b.message(t, cmd, "")

	b.dispatch(t, msg)
	got := b.statuses(t, cmd)
	failed := b.count(t, `SELECT count(*) FROM events WHERE type = 'trade.failed'`)
	if msg.verdict != "ack" || len(got) != 1 || got[0] != "failed:source_cancelled" || failed != 1 ||
		!b.recordedMsg(t, msg) || len(b.jup.Sent("req-1")) != 0 {
		t.Fatalf("verdict %q, swaps %v, %d trade.failed, %d executes; want one source_cancelled swap, never sent",
			msg.verdict, got, failed, len(b.jup.Sent("req-1")))
	}
}

func TestTradeEngine_RecheckReadFailureFailsTheRowAndNaksForAFreshClaim(t *testing.T) {
	t.Parallel()
	b := newEngineBus(t)
	b.proposals.FailOnce("Status", errs.New(errs.CodeUpstreamUnavailable, "governance.Status"))
	cmd := b.buy()
	msg := b.message(t, cmd, "")

	b.dispatch(t, msg)
	again := b.redeliver(t, msg)
	got := b.statuses(t, cmd)
	if msg.verdict != "nak" || again.verdict != "ack" || len(got) != 2 ||
		got[0] != "failed:never_submitted" || got[1] != "confirmed" {
		t.Fatalf("verdicts %q, %q, swaps %v; want a nak that frees the claim, then a confirmed retry",
			msg.verdict, again.verdict, got)
	}
}

func TestTradeEngine_JupiterDown_Naks(t *testing.T) {
	t.Parallel()
	b := newEngineBus(t)
	b.jup.Fail("Quote", errs.New(errs.CodeJupiterUnavailable, "jupiter.Quote"))
	cmd := b.buy()
	msg := b.message(t, cmd, "")

	b.dispatch(t, msg)
	if msg.verdict != "nak" || len(b.blocked(t, cmd.ProposalID)) != 0 || b.recordedMsg(t, msg) {
		t.Fatalf("verdict %q, %d blocked, recorded %v; want a nak with no trade.blocked and no delivery row",
			msg.verdict, len(b.blocked(t, cmd.ProposalID)), b.recordedMsg(t, msg))
	}
}

type slowVenue struct {
	app.Venue
	run func()
}

func (v slowVenue) ExecuteUntilTerminal(
	ctx context.Context, requestID string, signed []byte,
) (app.ExecuteResult, error) {
	v.run()
	return v.Venue.ExecuteUntilTerminal(ctx, requestID, signed)
}

func TestTradeEngine_InProgressDuringSwap(t *testing.T) {
	t.Parallel()
	var msg *engineMsg
	b := newEngineBus(t, func(e *engineEnv) {
		e.venue = slowVenue{Venue: e.venue, run: func() {
			deadline := time.NewTimer(10 * time.Second)
			defer deadline.Stop()
			for range 9 {
				e.clk.Advance(10 * time.Second)
				select {
				case <-msg.progressed:
				case <-deadline.C:
					return
				}
			}
		}}
	})
	cmd := b.buy()
	msg = b.message(t, cmd, "")
	msg.progressed = make(chan struct{}, 16)

	b.dispatch(t, msg)
	got := b.statuses(t, cmd)
	if n := msg.inProgress.Load(); n < 8 || msg.verdict != "ack" || len(got) != 1 || got[0] != "confirmed" {
		t.Fatalf("%d InProgress over a 90 s execute, verdict %q, swaps %v; want at least 8, one ack, one swap",
			n, msg.verdict, got)
	}
}
