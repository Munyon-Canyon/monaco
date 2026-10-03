package funding_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow05_CreditDeposit_OK(t *testing.T) {
	t.Parallel()
	flows.F05CreditDepositOK(flow05Scenario(t))
}

func TestFlow05_CreditDeposit_RPCUnavailable(t *testing.T) {
	t.Parallel()
	flows.F05CreditDepositRPCUnavailable(flow05Scenario(t))
}

func flow05Scenario(t *testing.T) *scenario.Scenario {
	t.Helper()
	upstreams := fakes.New()
	srv := httptest.NewServer(upstreams)
	t.Cleanup(srv.Close)
	return scenario.New(t,
		scenario.WithPrivy(upstreams, "app"),
		scenario.WithModules(func(d module.Deps) module.Module {
			d.Config = config.Config{
				Solana:   config.Solana{RPCURL: srv.URL + "/rpc/", USDCMint: string(testkit.USDCMint)},
				Funding:  config.Funding{DepositPollInterval: time.Second, DepositRPCRate: 1000},
				Timeouts: config.Timeouts{RPC: time.Second},
			}
			d.HTTPClient = httpclient.New
			return funding.New(d)
		}, func(d module.Deps) module.Module {
			d.Config.Solana.USDCMint = string(testkit.USDCMint)
			return treasury.New(d)
		}),
	)
}
