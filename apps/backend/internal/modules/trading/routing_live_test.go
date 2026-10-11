package trading_test

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestModule_aRoutedSwapUsesTheCabalsTreasuryEVMWallet(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	seeded := testkit.NewCabal(t, e.pool)
	e.cabal = seeded.ID
	e.seedCabal(cabal.StatusActive, 100)
	e.mustBuy(t, e.request(), domain.StatusCreated)
	e.clk.Advance(app.SwapSweepAge + time.Second)
	srv := httptest.NewServer(fakes.New())
	t.Cleanup(srv.Close)
	cfg := testkit.Config()
	cfg.Solana.RPCURL, cfg.Timeouts.RPC, cfg.Timeouts.Privy = "http://fakes/rpc/", time.Second, time.Second
	cfg.Routing = config.Routing{
		UniswapAPIKey: "key", UniswapBaseURL: srv.URL + "/uniswap", UniswapTimeout: time.Second,
	}
	cfg.EVM = config.EVM{RPCURLBase: srv.URL + "/base"}
	cfg.Privy = config.Privy{
		AppID: "app", AppSecret: "privy-app-5ecret", BaseURL: srv.URL + "/privy",
		VerificationKey:         fakes.PrivyVerificationKey(),
		AuthorizationPrivateKey: fakes.PrivyAuthorizationKeyConfig(),
		AuthorizationKeyID:      fakes.PrivyAuthorizationKeyID,
	}
	d := module.Deps{Config: cfg, Clock: e.clk, IDs: e.ids, Pool: e.pool, UoW: e.uow}
	m := trading.New(
		d,
		trading.WithEnginePorts(e.ports()),
		trading.WithChain(e.venue, e.signer),
		trading.WithChainReader(e.fills),
	)
	module.NewSet(cabal.New(d), m)

	_, err := m.Pollers()[1].Tick(actorContext(t.Context()))

	if errs.CodeOf(err) != errs.CodeAssetUntradable || !strings.Contains(err.Error(), "market.Catalog.Listing") {
		t.Fatalf("Tick err = %v, want the market catalog refusing the unverified listing", err)
	}
	if n := e.count(t, `SELECT count(*) FROM treasury_evm_wallets WHERE cabal_id = $1`, seeded.ID.UUID()); n != 1 {
		t.Fatalf("%d treasury EVM wallets, want the cabal's one from the wired port", n)
	}
}

func TestModule_routesOnEVMOnlyWhenTheVenueQuoterAndTreasuryWalletsAreLive(t *testing.T) {
	t.Parallel()
	keyed := testkit.Config()
	keyed.Routing = config.Routing{
		UniswapAPIKey: "key", UniswapBaseURL: "https://uniswap.test", UniswapTimeout: time.Second,
	}
	keyless := testkit.Config()
	for name, c := range map[string]struct {
		cfg     config.Config
		opts    []trading.Option
		noCabal bool
		want    bool
	}{
		"all live":                {cfg: keyed, want: true},
		"venue unconfigured":      {cfg: keyless},
		"no treasury wallet port": {cfg: keyed, noCabal: true},
		"quoter not market's": {
			cfg: keyed, opts: []trading.Option{trading.WithRoutePorts(app.RoutePorts{Quoter: multichain.Pending{}})},
		},
		"venue not live": {
			cfg: keyed, opts: []trading.Option{trading.WithRoutePorts(app.RoutePorts{Venue: app.UnwiredEVMVenue{}})},
		},
	} {
		d := module.Deps{Config: c.cfg, Clock: testkit.NewClock(time.Unix(0, 0))}
		m := trading.New(d, c.opts...)
		if !c.noCabal {
			module.NewSet(cabal.New(d), m)
		}
		if got := m.RoutesOnEVM(); got != c.want {
			t.Errorf("%s: routes on EVM = %v, want %v", name, got, c.want)
		}
	}
}

func TestModule_aMissingTreasuryWalletPortRefusesInsteadOfPanicking(t *testing.T) {
	t.Parallel()
	m := trading.New(module.Deps{Config: testkit.Config(), Clock: testkit.NewClock(time.Unix(0, 0))},
		trading.WithRoutePorts(app.RoutePorts{EVM: true}))

	_, err := m.RouteTreasuryEVM().EnsureTreasuryEVMWallet(t.Context(), ids.CabalID{})

	if errs.CodeOf(err) != errs.CodeNotImplemented {
		t.Fatalf("EnsureTreasuryEVMWallet err = %v, want not_implemented", err)
	}
}
