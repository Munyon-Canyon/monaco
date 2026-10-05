package trading_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestModule_ordersNameTheConfiguredRelayerAsPayer(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	srv := httptest.NewServer(fakes.New())
	t.Cleanup(srv.Close)
	cfg := testkit.Config()
	cfg.Privy = config.Privy{
		AppID: "app", AppSecret: "privy-app-5ecret", BaseURL: srv.URL + "/privy",
		VerificationKey:         fakes.PrivyVerificationKey(),
		AuthorizationPrivateKey: fakes.PrivyAuthorizationKeyConfig(),
		AuthorizationKeyID:      fakes.PrivyAuthorizationKeyID,
	}
	cfg.Relayer = config.Relayer{PrivateKey: chain.EncodeBase58(fakes.FixtureKey("chainfake-relayer"))}
	cfg.Solana.RPCURL = srv.URL + "/rpc/"
	cfg.Timeouts = config.Timeouts{Privy: time.Second, RPC: time.Second}
	orders := &orderLog{Venue: e.venue}
	conn := testkit.NATS(t).Conn
	m := trading.New(module.Deps{Config: cfg, Clock: e.clk, IDs: e.ids, Pool: e.pool, UoW: e.uow, Bus: conn},
		trading.WithEnginePorts(e.ports()), trading.WithChain(orders, nil))
	e.dispatchVia(t, conn, m)
	if len(orders.specs) == 0 || orders.specs[0].Payer != chainfake.RelayerAddress() {
		t.Fatalf("orders = %+v, want the configured relayer %s as payer", orders.specs, chainfake.RelayerAddress())
	}
}
