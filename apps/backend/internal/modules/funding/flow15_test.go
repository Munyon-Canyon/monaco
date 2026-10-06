package funding_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
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

func TestFlow15_Withdraw_OK(t *testing.T) {
	t.Parallel()
	flows.F15WithdrawOK(flow15Scenario(t, scenario.WithPostHog(t)))
}

func TestFlow15_Withdraw_InvalidInput(t *testing.T) {
	t.Parallel()
	flows.F15WithdrawInvalidInput(flow15Scenario(t))
}

func TestFlow15_Withdraw_InvalidAddress(t *testing.T) {
	t.Parallel()
	flows.F15WithdrawInvalidAddress(flow15Scenario(t))
}

func TestFlow15_Withdraw_WithdrawToOwnWallet(t *testing.T) {
	t.Parallel()
	flows.F15WithdrawWithdrawToOwnWallet(flow15Scenario(t))
}

func TestFlow15_Withdraw_InsufficientFunds(t *testing.T) {
	t.Parallel()
	flows.F15WithdrawInsufficientFunds(flow15Scenario(t))
}

func TestFlow15_Withdraw_PrivyUnavailable(t *testing.T) {
	t.Parallel()
	flows.F15WithdrawPrivyUnavailable(flow15Scenario(t))
}

func TestWithdraw_aBannedMemberCanStillWithdraw(t *testing.T) {
	t.Parallel()
	s := flow15Scenario(t)
	member := flows.SeedWithdrawer(s)
	if _, err := s.DB().Exec(t.Context(), `UPDATE users SET account_status = 'banned' WHERE id = $1`,
		member.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	s.Given(scenario.AsSeededUser("member", member.ID)).
		When(scenario.Post("/v1/me/withdrawals",
			`{"amount_micros":"2000000","to_address":"9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P"}`)).
		Then(scenario.ExpectStatus(http.StatusAccepted), scenario.ExpectEvents(events.TypeWithdrawalSubmitted, 1))
}

func flow15Scenario(t *testing.T, extra ...scenario.Option) *scenario.Scenario {
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
		Relayer:  config.Relayer{PrivateKey: chain.EncodeBase58(fakes.FixtureKey("flow15-relayer"))},
		Funding:  config.Funding{DepositPollInterval: time.Minute, DepositRPCRate: 1000},
		Timeouts: config.Timeouts{RPC: time.Second, Privy: time.Second},
	}
	withConfig := func(build func(module.Deps) module.Module) func(module.Deps) module.Module {
		return func(d module.Deps) module.Module {
			d.Config, d.HTTPClient = cfg, httpclient.New
			return build(d)
		}
	}
	return scenario.New(t, append([]scenario.Option{
		scenario.WithPrivy(upstreams, "app"),
		scenario.WithModules(
			withConfig(func(d module.Deps) module.Module { return funding.New(d) }),
			withConfig(func(d module.Deps) module.Module { return treasury.New(d) }),
			withConfig(func(d module.Deps) module.Module { return cabal.New(d) }),
			withConfig(func(d module.Deps) module.Module { return identity.New(d) }),
		),
	}, extra...)...)
}
