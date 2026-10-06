package cabal_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const privyAppID = "app-fixture"

func cabalScenario(t *testing.T, extra ...scenario.Option) *scenario.Scenario {
	t.Helper()
	return cabalScenarioWith(t, nil, extra...)
}

func cabalScenarioWith(t *testing.T, opts []cabal.Option, extra ...scenario.Option) *scenario.Scenario {
	t.Helper()
	upstreams := fakes.New()
	srv := httptest.NewServer(upstreams)
	t.Cleanup(srv.Close)
	cfg := config.Config{
		Privy: config.Privy{
			AppID: privyAppID, AppSecret: "test-secret", BaseURL: srv.URL + "/privy",
			VerificationKey:         fakes.PrivyVerificationKey(),
			AuthorizationPrivateKey: fakes.PrivyAuthorizationKeyConfig(),
			AuthorizationKeyID:      fakes.PrivyAuthorizationKeyID,
		},
		Solana:   config.Solana{USDCMint: string(testkit.USDCMint)},
		Timeouts: config.Timeouts{Privy: 10 * time.Second},
	}
	withCfg := func(build func(module.Deps) module.Module) func(module.Deps) module.Module {
		return func(d module.Deps) module.Module {
			d.Config = cfg
			return build(d)
		}
	}
	return scenario.New(t, append([]scenario.Option{
		scenario.WithModules(
			withCfg(func(d module.Deps) module.Module { return identity.New(d) }),
			withCfg(func(d module.Deps) module.Module { return cabal.New(d, opts...) }),
		),
		scenario.WithPrivy(upstreams, privyAppID),
	}, extra...)...)
}

func TestFlow02_CreateCabal_OK(t *testing.T) {
	t.Parallel()
	flows.F02CreateCabalOK(cabalScenario(t, scenario.WithPostHog(t)))
}

func TestFlow02_CreateCabal_InvalidInput(t *testing.T) {
	t.Parallel()
	flows.F02CreateCabalInvalidInput(cabalScenario(t))
}

func TestFlow02_CreateCabal_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F02CreateCabalUnauthorized(cabalScenario(t))
}

func TestFlow02_CreateCabal_PrivyUnavailable(t *testing.T) {
	t.Parallel()
	flows.F02CreateCabalPrivyUnavailable(cabalScenario(t))
}
