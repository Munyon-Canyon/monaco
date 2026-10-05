package treasury_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow07_FundCabal_OK(t *testing.T) {
	t.Parallel()
	flows.F07FundCabalOK(flow07Scenario(t))
}

func TestFlow07_FundCabal_InvalidInput(t *testing.T) {
	t.Parallel()
	flows.F07FundCabalInvalidInput(flow07Scenario(t))
}

func TestFlow07_FundCabal_NotCabalMember(t *testing.T) {
	t.Parallel()
	flows.F07FundCabalNotCabalMember(flow07Scenario(t))
}

func TestFlow07_FundCabal_InsufficientFunds(t *testing.T) {
	t.Parallel()
	flows.F07FundCabalInsufficientFunds(flow07Scenario(t))
}

func TestFlow07_FundCabal_CabalPaused(t *testing.T) {
	t.Parallel()
	flows.F07FundCabalCabalPaused(flow07Scenario(t))
}

func TestFlow07_FundCabal_PotValueZero(t *testing.T) {
	t.Parallel()
	flows.F07FundCabalPotValueZero(flow07Scenario(t))
}

func TestFlow07_FundCabal_PrivyUnavailable(t *testing.T) {
	t.Parallel()
	flows.F07FundCabalPrivyUnavailable(flow07Scenario(t))
}

func flow07Scenario(t *testing.T) *scenario.Scenario {
	t.Helper()
	upstreams := fakes.New()
	srv := httptest.NewServer(upstreams)
	t.Cleanup(srv.Close)
	cfg := config.Config{
		Solana: config.Solana{RPCURL: srv.URL + "/rpc/", USDCMint: string(testkit.USDCMint)},
		Privy: config.Privy{
			AppID: "app", AppSecret: "privy-app-5ecret", BaseURL: srv.URL + "/privy",
			VerificationKey:         fakes.PrivyVerificationKey(),
			AuthorizationPrivateKey: fakes.PrivyAuthorizationKeyConfig(),
			AuthorizationKeyID:      fakes.PrivyAuthorizationKeyID,
		},
		Relayer:  config.Relayer{PrivateKey: chain.EncodeBase58(fakes.FixtureKey("flow07-relayer"))},
		Funding:  config.Funding{DepositPollInterval: time.Minute, DepositRPCRate: 1000},
		Timeouts: config.Timeouts{RPC: time.Second, Privy: time.Second},
	}
	withConfig := func(build func(module.Deps) module.Module) func(module.Deps) module.Module {
		return func(d module.Deps) module.Module {
			d.Config, d.HTTPClient = cfg, httpclient.New
			return build(d)
		}
	}
	return scenario.New(t,
		scenario.WithPrivy(upstreams, "app"),
		scenario.WithModules(
			withConfig(func(d module.Deps) module.Module { return treasury.New(d) }),
			withConfig(func(d module.Deps) module.Module { return funding.New(d) }),
			withConfig(func(d module.Deps) module.Module { return cabal.New(d) }),
			withConfig(func(d module.Deps) module.Module { return identity.New(d) }),
		),
	)
}
