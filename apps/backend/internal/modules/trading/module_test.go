package trading_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	busevents "github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	governance "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestModule_registersTheEngineAsItsOwnIdempotentConsumer(t *testing.T) {
	t.Parallel()
	m := trading.New(module.Deps{Config: config.Config{
		Solana: config.Solana{RPCURL: "http://fakes/rpc/"}, Timeouts: config.Timeouts{RPC: time.Second},
	}})
	consumers := m.Consumers()
	if m.Name() != "trading" || len(m.Pollers()) != 1 || testkit.Serves(m.Mount, "GET", "/v1/assets") ||
		len(consumers) != 1 {
		t.Fatalf("module = %s, %d consumers, %v pollers", m.Name(), len(consumers), m.Pollers())
	}
	if reflect.ValueOf(m.SignatureOwner()).IsZero() {
		t.Fatal("SignatureOwner() = zero value")
	}
	c := consumers[0]
	type handler struct {
		name string
		typ  busevents.Type
		own  bool
	}
	got := make([]handler, 0, len(c.Handlers))
	for _, h := range c.Handlers {
		got = append(got, handler{h.Name, h.Type(), h.OwnIdempotency()})
	}
	want := []handler{
		{engineHandler, busevents.TypeProposalPassed, true}, {retryHandler, busevents.TypeTradeRetryRequested, true},
	}
	if c.Durable != "trading" || !reflect.DeepEqual(got, want) {
		t.Fatalf("consumer %s handlers %+v, want durable trading with %+v", c.Durable, got, want)
	}
}

type provider struct {
	name    string
	cabals  cabalport.Queries
	govern  governance.Queries
	pausing fundingport.Pauses
}

func (p provider) Name() string            { return p.name }
func (provider) Mount(api.Mount)           {}
func (provider) Consumers() []bus.Consumer { return nil }
func (provider) Pollers() []poller.Poller  { return nil }

type cabalModule struct{ provider }

func (c cabalModule) Queries() cabalport.Queries { return c.cabals }

type governanceModule struct{ provider }

func (g governanceModule) Queries() governance.Queries { return g.govern }

type fundingModule struct{ provider }

func (f fundingModule) Pauses() fundingport.Pauses { return f.pausing }

func (e *engineEnv) moduleWith(t *testing.T, conn *bus.Conn, opts ...trading.Option) *trading.Module {
	t.Helper()
	return trading.New(module.Deps{
		Config: testkit.Config(), Clock: e.clk, IDs: e.ids, Pool: e.pool, UoW: e.uow, Bus: conn,
	}, opts...)
}

func (e *engineEnv) dispatchVia(t *testing.T, conn *bus.Conn, m *trading.Module) (*engineMsg, []string) {
	t.Helper()
	reg, err := bus.NewRegistry(conn, e.uow, e.clk, m.Consumers())
	if err != nil {
		t.Fatal(err)
	}
	b := &engineBus{engineEnv: e, conn: conn, reg: reg}
	cmd := e.buy()
	msg := b.message(t, cmd, "")
	b.dispatch(t, msg)
	return msg, b.statuses(t, cmd)
}

func TestModule_wireFillsTheUnsetPortsFromTheSetAndKeepsTheInjectedOnes(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	conn := testkit.NATS(t).Conn
	ports := e.ports()
	ports.Cabals, ports.Proposals, ports.Pauses = nil, nil, nil
	m := e.moduleWith(t, conn, trading.WithEnginePorts(ports), trading.WithChain(e.venue, e.signer))
	module.NewSet(m,
		cabalModule{provider{name: "cabal", cabals: e.cabals}},
		governanceModule{provider{name: "governance", govern: e.proposals}},
		fundingModule{provider{name: "funding", pausing: e.pauses}},
	)

	msg, swaps := e.dispatchVia(t, conn, m)
	if msg.verdict != "ack" || len(swaps) != 1 || swaps[0] != "confirmed" {
		t.Fatalf("verdict %q, swaps %v; want the wired ports to carry the trade to confirmed", msg.verdict, swaps)
	}

	paused := fakes.NewPauses()
	paused.Pause(e.cabal, funding.PauseReasonOps)
	ports.Pauses = paused
	m = e.moduleWith(t, conn, trading.WithEnginePorts(ports), trading.WithChain(e.venue, e.signer))
	module.NewSet(m, fundingModule{provider{name: "funding", pausing: e.pauses}},
		cabalModule{provider{name: "cabal", cabals: e.cabals}},
		governanceModule{provider{name: "governance", govern: e.proposals}})
	msg, swaps = e.dispatchVia(t, conn, m)
	if msg.verdict != "ack" || len(swaps) != 0 {
		t.Fatalf("verdict %q, swaps %v; want the injected pause to win over the set's and block", msg.verdict, swaps)
	}
}

func TestModule_unwiredPortsNakTheTrade(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	conn := testkit.NATS(t).Conn
	ports := e.ports()
	ports.Cabals = nil
	m := e.moduleWith(t, conn, trading.WithEnginePorts(ports), trading.WithChain(e.venue, e.signer))

	msg, swaps := e.dispatchVia(t, conn, m)
	if msg.verdict != "nak" || len(swaps) != 0 {
		t.Fatalf("verdict %q, swaps %v; want an unwired cabal port to nak before any swap", msg.verdict, swaps)
	}
}

func TestModule_defaultsReadTheCatalogTableAndBlockAnUnknownAsset(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	conn := testkit.NATS(t).Conn
	cfg := testkit.Config()
	cfg.Jupiter = config.Jupiter{SwapBaseURL: "http://127.0.0.1:1", PriceBaseURL: "http://127.0.0.1:1"}
	cfg.Solana.RPCURL = "http://127.0.0.1:1"
	cfg.Privy = config.Privy{VerificationKey: fakes.PrivyVerificationKey(), BaseURL: "http://127.0.0.1:1"}
	cfg.Timeouts = config.Timeouts{
		JupiterQuote: time.Second, JupiterExecute: time.Second, RPC: time.Second, Privy: time.Second,
	}
	m := trading.New(module.Deps{Config: cfg, Clock: e.clk, IDs: e.ids, Pool: e.pool, UoW: e.uow, Bus: conn})

	msg, swaps := e.dispatchVia(t, conn, m)
	blocked := e.count(t, `SELECT count(*) FROM events WHERE type = 'trade.blocked' AND payload->>'code' = $1`,
		string(errs.CodeAssetUntradable))
	if msg.verdict != "ack" || len(swaps) != 0 || blocked != 1 {
		t.Fatalf("verdict %q, swaps %v, %d blocked; want the empty catalog table to block as asset_untradable",
			msg.verdict, swaps, blocked)
	}
}

func TestModule_withoutPrivyConfigTheTradeTermsAtSigning(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	conn := testkit.NATS(t).Conn
	m := e.moduleWith(t, conn, trading.WithEnginePorts(e.ports()), trading.WithChain(e.venue, nil))

	msg, swaps := e.dispatchVia(t, conn, m)
	if msg.verdict != "term" || len(swaps) != 1 || swaps[0] != "created" {
		t.Fatalf("verdict %q, swaps %v; want a term with the swap left created and never signed", msg.verdict, swaps)
	}
}
