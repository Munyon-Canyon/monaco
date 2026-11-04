package referrals_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
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

func referralScenario(t *testing.T, extra ...scenario.Option) *scenario.Scenario {
	t.Helper()
	return scenario.New(t, append([]scenario.Option{
		scenario.WithModules(func(d module.Deps) module.Module { return referrals.New(d) }),
	}, extra...)...)
}

func qualifyScenario(t *testing.T) *scenario.Scenario {
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
		Relayer:  config.Relayer{PrivateKey: chain.EncodeBase58(fakes.FixtureKey("flow25-relayer"))},
		Funding:  config.Funding{DepositPollInterval: time.Minute, DepositRPCRate: 1000},
		Timeouts: config.Timeouts{RPC: time.Second, Privy: time.Second},
	}
	withConfig := func(build func(module.Deps) module.Module) func(module.Deps) module.Module {
		return func(d module.Deps) module.Module {
			d.Config, d.HTTPClient = cfg, httpclient.New
			return build(d)
		}
	}
	return referralScenario(t,
		scenario.WithPostHog(t),
		scenario.WithPrivy(upstreams, "app"),
		scenario.WithModules(
			withConfig(func(d module.Deps) module.Module { return treasury.New(d) }),
			withConfig(func(d module.Deps) module.Module { return funding.New(d) }),
			withConfig(func(d module.Deps) module.Module { return cabal.New(d) }),
			withConfig(func(d module.Deps) module.Module { return identity.New(d) }),
		))
}

func TestFlow25_AttachReferral_OK(t *testing.T) {
	t.Parallel()
	flows.F25AttachReferralOK(qualifyScenario(t))
}

func TestFlow25_AttachReferral_ReferralCodeUnknown(t *testing.T) {
	t.Parallel()
	flows.F25AttachReferralReferralCodeUnknown(referralScenario(t))
}

func TestFlow25_AttachReferral_ReferralSelf(t *testing.T) {
	t.Parallel()
	flows.F25AttachReferralReferralSelf(referralScenario(t))
}

func TestFlow25_AttachReferral_ReferralAlreadyAttached(t *testing.T) {
	t.Parallel()
	flows.F25AttachReferralReferralAlreadyAttached(referralScenario(t))
}

func TestFlow25_AttachReferral_ReferralWindowClosed(t *testing.T) {
	t.Parallel()
	flows.F25AttachReferralReferralWindowClosed(referralScenario(t))
}

func TestFlow25_AttachReferral_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F25AttachReferralUnauthorized(referralScenario(t))
}

func TestFlow25_AttachReferral_RateLimited(t *testing.T) {
	t.Parallel()
	flows.F25AttachReferralRateLimited(referralScenario(t))
}

func TestAttachReferral_Ok_SocialFollowsBothWays(t *testing.T) {
	t.Parallel()
	flows.F25AttachReferralSocialFollowsBothWays(referralScenario(t,
		scenario.WithModules(func(d module.Deps) module.Module { return social.New(d) })))
}
