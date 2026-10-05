package trading_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	busevents "github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chaos"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const cashOutSellHandler = "trading.cashout_sell"

type ledgerPositions []treasuryport.Position

func (p ledgerPositions) Positions(context.Context, ids.CabalID) ([]treasuryport.Position, error) {
	return p, nil
}

type cashOutBus struct {
	*cashOutEnv
	reg  *bus.Registry
	conn *bus.Conn
}

func newCashOutBus(t *testing.T, arrange ...func(*config.Config, *app.EnginePorts)) *cashOutBus {
	t.Helper()
	c := newCashOutEnv(t)
	wallet := cabal.TreasuryWallet{
		CabalID: c.cabal, PrivyWalletID: treasuryWallet, Address: chainfake.WalletAddress(treasuryWallet),
	}
	ports := app.EnginePorts{
		Catalog: marketfake.NewCatalog(marketfake.Fixtures()...),
		Cabals:  fakes.NewCabal([]fakes.CabalSeed{{View: cabal.View{ID: c.cabal}, Wallet: wallet}}, nil),
		Positions: ledgerPositions{
			{Mint: usdcMint, Units: money.NewBaseUnits(900_000_000, 6)},
			{Mint: tslaxMint, Units: money.NewBaseUnits(300, 8)},
			{Mint: aaplxMint, Units: money.NewBaseUnits(1_000, 8)},
		},
	}
	conn := testkit.NATS(t).Conn
	cfg := testkit.Config()
	cfg.Solana.RPCURL, cfg.Timeouts.RPC = "http://127.0.0.1:1", time.Second
	for _, a := range arrange {
		a(&cfg, &ports)
	}
	m := trading.New(module.Deps{
		Config: cfg, Clock: c.clk, IDs: c.ids, Pool: c.pool, UoW: c.uow, Bus: conn,
	}, trading.WithEnginePorts(ports), trading.WithChain(c.venue, c.signer))
	reg, err := bus.NewRegistry(conn, c.uow, c.clk, m.Consumers())
	if err != nil {
		t.Fatal(err)
	}
	return &cashOutBus{cashOutEnv: c, reg: reg, conn: conn}
}

func (b *cashOutBus) started(t *testing.T, sellMicros uint64) *engineMsg {
	t.Helper()
	ev := busevents.CashOutStarted{
		V: 1, JobID: b.job, CabalID: b.cabal.UUID(), UserID: b.ids.NewV7(), ShareUnits: 10,
		PayoutMicros: money.MicrosFromUint64(sellMicros + 1), SellUSDC: money.MicrosFromUint64(sellMicros),
	}
	err := b.uow.Do(actorContext(t.Context()), func(ctx context.Context, tx db.Tx) error {
		return tx.Events.Append(ctx, ev)
	})
	var id uuid.UUID
	var payload json.RawMessage
	if err == nil {
		err = b.pool.QueryRow(t.Context(), `SELECT id, payload FROM events WHERE type = 'cashout.started'
			AND aggregate_id = $1`, b.job).Scan(&id, &payload)
	}
	if err != nil {
		t.Fatal(err)
	}
	eid := ids.EventIDFrom(id)
	return &engineMsg{
		Msg: chaos.NewMsg(b.conn, busevents.TypeCashOutStarted, eid, payload), id: eid, payload: payload,
		delivered: 1,
	}
}

func (b *cashOutBus) redeliver(msg *engineMsg) *engineMsg {
	return &engineMsg{
		Msg: chaos.NewMsg(b.conn, busevents.TypeCashOutStarted, msg.id, msg.payload), id: msg.id,
		payload: msg.payload, delivered: msg.delivered + 1,
	}
}

func (b *cashOutBus) dispatch(t *testing.T, msg *engineMsg) {
	t.Helper()
	b.reg.Dispatch(t.Context(), "trading_cashout_sell", msg)
}

func (b *cashOutBus) recorded(t *testing.T, msg *engineMsg) int {
	t.Helper()
	return b.recordedAs(t, msg, "ok")
}

func (b *cashOutBus) recordedAs(t *testing.T, msg *engineMsg, code string) int {
	t.Helper()
	var n int
	if err := b.pool.QueryRow(t.Context(), `SELECT count(*) FROM event_deliveries WHERE handler = $1
		AND event_id = $2 AND code = $3`, cashOutSellHandler, msg.id.UUID(), code).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

type cashOutTrade struct {
	Source          busevents.TradeSource `json:"source"`
	SourceBatchSize int                   `json:"source_batch_size"`
	Symbol          string                `json:"symbol"`
	OutAmount       string                `json:"out_amount"`
}

func (b *cashOutBus) confirmed(t *testing.T) []cashOutTrade {
	t.Helper()
	rows, err := b.pool.Query(t.Context(), `SELECT payload FROM events WHERE type = 'trade.confirmed'
		AND payload->'source'->>'id' = $1 ORDER BY id`, b.job.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []cashOutTrade
	for rows.Next() {
		var raw []byte
		var tr cashOutTrade
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &tr); err != nil {
			t.Fatal(err)
		}
		out = append(out, tr)
	}
	return out
}

func TestCashOutSell_StartedWithAShortfall_SellsAndConfirmsEachLegAsOneBatch(t *testing.T) {
	t.Parallel()
	b := newCashOutBus(t)
	msg := b.started(t, 60_000_000)

	b.dispatch(t, msg)
	if msg.verdict != "ack" || b.recorded(t, msg) != 1 {
		t.Fatalf("verdict %q, %d deliveries recorded; want an ack recorded once", msg.verdict, b.recorded(t, msg))
	}
	assertLegs(t, b.legs(t), confirmedLeg("AAPLx", 1_000, 50_000_000), confirmedLeg("TSLAx", 106, 10_600_000))
	src := busevents.TradeSource{Kind: "cashout", ID: b.job}
	want := []cashOutTrade{
		{Source: src, SourceBatchSize: 2, Symbol: "AAPLx", OutAmount: "49800000"},
		{Source: src, SourceBatchSize: 2, Symbol: "TSLAx", OutAmount: "10550000"},
	}
	if got := b.confirmed(t); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("trade.confirmed = %+v\nwant %+v", got, want)
	}

	again := b.redeliver(msg)
	b.dispatch(t, again)
	if again.verdict != "ack" || len(b.legs(t)) != 2 || len(b.confirmed(t)) != 2 {
		t.Fatalf("redelivery verdict %q, %d legs, %d confirmed; want an ack and no second sale",
			again.verdict, len(b.legs(t)), len(b.confirmed(t)))
	}
}

func TestCashOutSell_StartedCovered_SellsNothing(t *testing.T) {
	t.Parallel()
	b := newCashOutBus(t)
	msg := b.started(t, 0)

	b.dispatch(t, msg)
	if msg.verdict != "ack" || b.recorded(t, msg) != 1 || len(b.legs(t)) != 0 || b.plan(t) != "" {
		t.Fatalf("verdict %q, %d recorded, legs %+v, plan %q; want an ack and no sale", msg.verdict,
			b.recorded(t, msg), b.legs(t), b.plan(t))
	}
}

func TestModule_registersTheCashOutSellAsItsOwnIdempotentDurable(t *testing.T) {
	t.Parallel()
	sell := trading.New(module.Deps{Config: testkit.Config()}).Consumers()[1]
	if sell.Durable != "trading_cashout_sell" || len(sell.Handlers) != 1 {
		t.Fatalf("consumer %s with %d handlers, want durable trading_cashout_sell with one", sell.Durable,
			len(sell.Handlers))
	}
	if h := sell.Handlers[0]; h.Name != cashOutSellHandler || h.Type() != busevents.TypeCashOutStarted ||
		!h.OwnIdempotency() {
		t.Fatalf("handler %s on %s own=%v, want %s on cashout.started owning its delivery", h.Name, h.Type(),
			h.OwnIdempotency(), cashOutSellHandler)
	}
}

func TestCashOutSell_StubEngine_RecordsWithoutSelling(t *testing.T) {
	t.Parallel()
	b := newCashOutBus(t, func(c *config.Config, _ *app.EnginePorts) { c.Trade.Engine = config.TradeEngineStub })
	msg := b.started(t, 60_000_000)

	b.dispatch(t, msg)
	if msg.verdict != "ack" || b.recordedAs(t, msg, "stubbed") != 1 || len(b.legs(t)) != 0 {
		t.Fatalf("verdict %q, %d stubbed, legs %+v; want the stub to record and sell nothing", msg.verdict,
			b.recordedAs(t, msg, "stubbed"), b.legs(t))
	}
	again := b.redeliver(msg)
	b.dispatch(t, again)
	if again.verdict != "ack" || b.recordedAs(t, msg, "stubbed") != 1 {
		t.Fatalf("redelivery verdict %q, %d stubbed; want an ack and the one stubbed row", again.verdict,
			b.recordedAs(t, msg, "stubbed"))
	}
}

func TestCashOutSell_PortFailures_NakOrTermBeforeAnySale(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeUpstreamUnavailable, "test")
	cases := []struct {
		name    string
		arrange func(*config.Config, *app.EnginePorts)
		verdict string
	}{
		{"positions unwired", func(_ *config.Config, p *app.EnginePorts) { p.Positions = nil }, "nak"},
		{"stored mint is not an address", func(_ *config.Config, p *app.EnginePorts) {
			p.Positions = ledgerPositions{{Mint: "not-a-mint", Units: money.NewBaseUnits(1, 8)}}
		}, "term"},
		{"mint missing from the catalog", func(_ *config.Config, p *app.EnginePorts) {
			p.Catalog = marketfake.NewCatalog()
		}, "term"},
		{"cabal port down", func(_ *config.Config, p *app.EnginePorts) {
			cabals := fakes.NewCabal(nil, nil)
			cabals.Fail("TreasuryWallet", down)
			p.Cabals = cabals
		}, "nak"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := newCashOutBus(t, tc.arrange)
			msg := b.started(t, 60_000_000)

			b.dispatch(t, msg)
			if msg.verdict != tc.verdict || b.recorded(t, msg) != 0 || len(b.legs(t)) != 0 {
				t.Fatalf("verdict %q, %d recorded, legs %+v; want %s with no sale and no delivery", msg.verdict,
					b.recorded(t, msg), b.legs(t), tc.verdict)
			}
		})
	}
}
