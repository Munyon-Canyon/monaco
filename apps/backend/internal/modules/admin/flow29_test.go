package admin_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/admin"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const flow29PrivyApp = "app-fixture"

func flow29(t *testing.T) *scenario.Scenario {
	t.Helper()
	upstreams := fakes.New()
	srv := httptest.NewServer(upstreams)
	t.Cleanup(srv.Close)
	cfg := config.Config{
		Privy: config.Privy{
			AppID: flow29PrivyApp, AppSecret: "test-secret", BaseURL: srv.URL + "/privy",
			VerificationKey:         fakes.PrivyVerificationKey(),
			AuthorizationPrivateKey: fakes.PrivyAuthorizationKeyConfig(),
			AuthorizationKeyID:      fakes.PrivyAuthorizationKeyID,
		},
		Solana:  config.Solana{RPCURL: srv.URL + "/rpc/", USDCMint: string(testkit.USDCMint)},
		Jupiter: config.Jupiter{SwapBaseURL: srv.URL + "/jupiter/swap/v2", PriceBaseURL: srv.URL + "/jupiter/price/v3"},
		Relayer: config.Relayer{PrivateKey: flows.CashOutRelayerKey()},
		Timeouts: config.Timeouts{
			RPC: time.Second, Privy: 10 * time.Second, JupiterQuote: time.Second, JupiterExecute: 10 * time.Second,
		},
	}
	with := func(build func(module.Deps) module.Module) func(module.Deps) module.Module {
		return func(d module.Deps) module.Module {
			d.Config, d.HTTPClient = cfg, httpclient.New
			return build(d)
		}
	}
	return scenario.New(t, scenario.WithModules(
		with(func(d module.Deps) module.Module { return identity.New(d) }),
		with(func(d module.Deps) module.Module { return cabal.New(d) }),
		with(func(d module.Deps) module.Module { return funding.New(d) }),
		with(func(d module.Deps) module.Module { return treasury.New(d) }),
		with(func(d module.Deps) module.Module { return trading.New(d) }),
		with(func(d module.Deps) module.Module { return governance.New(d) }),
		with(func(d module.Deps) module.Module { return social.New(d) }),
		with(func(d module.Deps) module.Module { return admin.New(d) }),
	), scenario.WithPrivy(upstreams, flow29PrivyApp))
}

func TestFlow29_RequestCabalBan_OK(t *testing.T) {
	t.Parallel()
	flows.F29RequestCabalBanOK(flow29(t))
}

func TestFlow29_RequestCabalBan_CabalNotActive(t *testing.T) {
	t.Parallel()
	flows.F29RequestCabalBanCabalNotActive(flow29(t))
}

func TestFlow29_ApproveCabalBan_OK(t *testing.T) {
	t.Parallel()
	flows.F29ApproveCabalBanOK(flow29(t))
}

func TestFlow29_ApproveCabalBan_SameApprover(t *testing.T) {
	t.Parallel()
	flows.F29ApproveCabalBanSameApprover(flow29(t))
}

func TestFlow29_ApproveCabalBan_ApprovalExpired(t *testing.T) {
	t.Parallel()
	flows.F29ApproveCabalBanApprovalExpired(flow29(t))
}

func TestCabalBanned_RefusesProposeFundJoin(t *testing.T) {
	t.Parallel()
	flows.BannedCabalRefusesProposeFundAndJoin(flow29(t))
}
