package treasury_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const flow14PrivyApp = "app-fixture"

func flow14(t *testing.T) *scenario.Scenario {
	t.Helper()
	upstreams := fakes.New()
	srv := httptest.NewServer(upstreams)
	t.Cleanup(srv.Close)
	cfg := config.Config{
		Privy: config.Privy{
			AppID: flow14PrivyApp, AppSecret: "test-secret", BaseURL: srv.URL + "/privy",
			VerificationKey:         fakes.PrivyVerificationKey(),
			AuthorizationPrivateKey: fakes.PrivyAuthorizationKeyConfig(),
			AuthorizationKeyID:      fakes.PrivyAuthorizationKeyID,
		},
		Solana:   config.Solana{RPCURL: srv.URL + "/rpc/", USDCMint: string(testkit.USDCMint)},
		Relayer:  config.Relayer{PrivateKey: flows.CashOutRelayerKey()},
		Timeouts: config.Timeouts{RPC: time.Second, Privy: 10 * time.Second},
	}
	with := func(build func(module.Deps) module.Module) func(module.Deps) module.Module {
		return func(d module.Deps) module.Module {
			d.Config, d.HTTPClient = cfg, httpclient.New
			return build(d)
		}
	}
	return scenario.New(t,
		scenario.WithModules(
			with(func(d module.Deps) module.Module { return identity.New(d) }),
			with(func(d module.Deps) module.Module { return cabal.New(d) }),
			with(func(d module.Deps) module.Module { return funding.New(d) }),
			with(func(d module.Deps) module.Module { return treasury.New(d) }),
		),
		scenario.WithPrivy(upstreams, flow14PrivyApp),
	)
}

func TestFlow14_CashOut_OK(t *testing.T) {
	t.Parallel()
	flows.F14CashOutOK(flow14(t))
}

func TestFlow14_CashOut_InvalidInput(t *testing.T) {
	t.Parallel()
	flows.F14CashOutInvalidInput(flow14(t))
}

func TestFlow14_CashOut_InsufficientShares(t *testing.T) {
	t.Parallel()
	flows.F14CashOutInsufficientShares(flow14(t))
}

func TestFlow14_CashOut_CashOutInProgress(t *testing.T) {
	t.Parallel()
	flows.F14CashOutCashOutInProgress(flow14(t))
}

func TestFlow14_CashOut_CabalPaused(t *testing.T) {
	t.Parallel()
	flows.F14CashOutCabalPaused(flow14(t))
}

func TestFlow14_CashOut_PriceUnavailable(t *testing.T) {
	t.Parallel()
	flows.F14CashOutPriceUnavailable(flow14(t))
}

func TestFlow14_CashOutPayouts_PrivyUnavailable(t *testing.T) {
	t.Parallel()
	flows.F14CashOutPayoutsPrivyUnavailable(flow14(t))
}

func TestFlow14_CashOutPayouts_RPCUnavailable(t *testing.T) {
	t.Parallel()
	flows.F14CashOutPayoutsRPCUnavailable(flow14(t))
}

func TestCashOutFlow_aPayoutRejectedOnChainFailsTheJobAndReturnsTheUnits(t *testing.T) {
	t.Parallel()
	flows.CashOutRejectedOnChain(flow14(t))
}
