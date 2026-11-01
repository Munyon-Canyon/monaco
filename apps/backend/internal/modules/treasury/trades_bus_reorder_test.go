package treasury_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type busDispatcher struct {
	t       *testing.T
	f       fixture
	reg     *bus.Registry
	conn    *bus.Conn
	durable string
}

func newBusDispatcher(t *testing.T, f fixture) busDispatcher {
	t.Helper()
	conn := testkit.NATS(t).Conn
	deps := module.Deps{Config: f.cfg, Pool: f.pool, IDs: f.ids, Clock: f.clock, Bus: conn}
	var trades bus.Consumer
	for _, c := range treasury.New(deps).Consumers() {
		if c.Durable == "treasury_trades" {
			trades = c
		}
	}
	reg, err := bus.NewRegistry(conn, f.uow, f.clock, []bus.Consumer{trades})
	if err != nil {
		t.Fatal(err)
	}
	return busDispatcher{t: t, f: f, reg: reg, conn: conn, durable: trades.Durable}
}

func (d busDispatcher) deliver(e events.Event) *testkit.Delivery {
	d.t.Helper()
	if err := d.f.uow.Do(observability.WithActor(d.f.ctx(), "system:trading.engine"),
		func(ctx context.Context, tx db.Tx) error { return tx.Events.Append(ctx, e) }); err != nil {
		d.t.Fatal(err)
	}
	var raw uuid.UUID
	var payload json.RawMessage
	if err := d.f.pool.QueryRow(d.t.Context(),
		`SELECT id, payload FROM events ORDER BY id DESC LIMIT 1`).Scan(&raw, &payload); err != nil {
		d.t.Fatal(err)
	}
	m := testkit.NewDelivery(d.conn, e.Type(), ids.EventIDFrom(raw), payload)
	d.redeliver(m)
	return m
}

func (d busDispatcher) redeliver(m *testkit.Delivery) {
	d.t.Helper()
	m.Delivered++
	d.reg.Dispatch(d.f.ctx(), d.durable, m)
}

func (d busDispatcher) mustAck(m *testkit.Delivery, what string) {
	d.t.Helper()
	if m.Outcome != bus.OutcomeAck {
		d.t.Fatalf("%s: outcome %s reason %q, want ack", what, m.Outcome, m.Reason)
	}
}

func (d busDispatcher) mustRetry(m *testkit.Delivery, what string) {
	d.t.Helper()
	if m.Outcome != bus.OutcomeNak {
		d.t.Fatalf("%s: outcome %s reason %q, want nak so the event is retried", what, m.Outcome, m.Reason)
	}
	letters, err := d.conn.DeadLetters(d.t.Context())
	if err != nil || len(letters) != 0 {
		d.t.Fatalf("%s: dead letters = %+v (err %v), want none while retries remain", what, letters, err)
	}
}

func TestTradesBus_aRoutedBuyConfirmedBeforeItsBridgeLandsIsRetriedAndBookedOnceTheBridgeLands(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	d := newBusDispatcher(t, f)
	cabal := f.funded(t)
	out := newBridgeRun(t, f, cabal, "bridge_out")
	for _, e := range []events.TradeStepChanged{
		out.event("pending", "", 10_000_000, 0),
		out.event("submitted", "privy-"+out.swap.String(), 10_000_000, 0),
	} {
		d.mustAck(d.deliver(e), e.Status)
	}

	buy := d.deliver(evmConfirmed(cabal, out.swap, 400_000_000))
	d.mustRetry(buy, "trade.confirmed before bridge_out succeeded")

	d.mustAck(d.deliver(out.event("succeeded", "privy-"+out.swap.String(), 10_000_000, 7_500)), "bridge_out landing")
	d.redeliver(buy)
	d.mustAck(buy, "trade.confirmed redelivered after the landing")

	wantBridgePositions(t, f,
		position{Asset: usdcMint, Units: "90000000", Cost: "90000000"},
		position{Asset: baseCash, Units: "0", Cost: "0"},
		position{Asset: baseAAPL, Units: "400000000", Cost: "10000000"},
	)
	f.wantClean(t, "buy booked after the late landing")
}

func TestTradesBus_bridgeBackStepsBeforeTheEVMSellAreRetriedAndTheLandingIsBooked(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	d := newBusDispatcher(t, f)
	cabal := f.funded(t)
	out := newBridgeRun(t, f, cabal, "bridge_out")
	for _, e := range []events.Event{
		out.event("pending", "", 10_000_000, 0),
		out.event("submitted", "privy-"+out.swap.String(), 10_000_000, 0),
		out.event("succeeded", "privy-"+out.swap.String(), 10_000_000, 7_500),
		evmConfirmed(cabal, out.swap, 400_000_000),
	} {
		d.mustAck(d.deliver(e), string(e.Type()))
	}

	sellSwap := f.ids.NewV7()
	sale := evmConfirmed(cabal, sellSwap, 0)
	sale.Action, sale.InMint, sale.InAmount, sale.OutMint, sale.OutAmount = "sell", baseAAPL, 400_000_000, baseCash, 9_900_000
	sale.USDCMicros, sale.FeeMicros = money.MicrosFromUint64(9_900_000), money.MicrosFromUint64(0)
	back := bridgeRun{f: f, cabal: cabal, swap: sellSwap, step: "bridge_back"}

	var waiting []*testkit.Delivery
	for _, e := range []events.TradeStepChanged{
		back.event("pending", "", 9_900_000, 0),
		back.event("submitted", "privy-back", 9_900_000, 0),
		back.event("succeeded", "privy-back", 9_900_000, 7_000),
	} {
		m := d.deliver(e)
		if e.Status != "pending" {
			d.mustRetry(m, "bridge_back "+e.Status+" before the sell is booked")
			waiting = append(waiting, m)
		}
	}
	d.mustAck(d.deliver(sale), "the late sale")
	for _, m := range waiting {
		d.redeliver(m)
		d.mustAck(m, "bridge_back redelivered after the sale")
	}

	wantBridgePositions(t, f,
		position{Asset: usdcMint, Units: "99893000", Cost: "99893000"},
		position{Asset: baseCash, Units: "0", Cost: "0"},
		position{Asset: baseAAPL, Units: "0", Cost: "0"},
	)
	if got := f.inTransit(t, cabal); len(got) != 0 {
		t.Fatalf("in transit = %+v, want the bridge_back landing booked", got)
	}
	f.wantClean(t, "bridge back booked after the late sale")
}
