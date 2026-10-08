package ranking_test

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

func TestCashOutStart_publishesLeaderboardsUpdated(t *testing.T) {
	t.Parallel()
	upstreams := fakes.New()
	srv := httptest.NewServer(upstreams)
	t.Cleanup(srv.Close)
	cfg := config.Config{}
	cfg.Privy = config.Privy{
		AppID: flow14PrivyApp, AppSecret: "test-secret", BaseURL: srv.URL + "/privy",
		VerificationKey:         fakes.PrivyVerificationKey(),
		AuthorizationPrivateKey: fakes.PrivyAuthorizationKeyConfig(),
		AuthorizationKeyID:      fakes.PrivyAuthorizationKeyID,
	}
	cfg.Solana = config.Solana{RPCURL: srv.URL + "/rpc/", USDCMint: string(testkit.USDCMint)}
	cfg.Relayer = config.Relayer{PrivateKey: flows.CashOutRelayerKey()}
	cfg.Jupiter = config.Jupiter{SwapBaseURL: srv.URL + "/jupiter/swap/v2", PriceBaseURL: srv.URL + "/jupiter/price/v3"}
	cfg.Timeouts = config.Timeouts{
		RPC: time.Second, Privy: 10 * time.Second, JupiterQuote: time.Second, JupiterExecute: 10 * time.Second,
	}
	with := func(build func(module.Deps) module.Module) func(module.Deps) module.Module {
		return func(d module.Deps) module.Module {
			d.Config, d.HTTPClient = cfg, httpclient.New
			return build(d)
		}
	}
	flows.CashOutStartRevalues(scenario.New(t,
		scenario.WithModules(
			with(func(d module.Deps) module.Module { return identity.New(d) }),
			with(func(d module.Deps) module.Module { return cabal.New(d) }),
			with(func(d module.Deps) module.Module { return funding.New(d) }),
			with(func(d module.Deps) module.Module { return treasury.New(d) }),
			with(func(d module.Deps) module.Module { return rankingOver(d) }),
		),
		scenario.WithPrivy(upstreams, flow14PrivyApp),
	))
}
