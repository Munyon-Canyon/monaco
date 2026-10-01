package identity_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	privyadapter "github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func identityScenario(t *testing.T, extra ...scenario.Option) *scenario.Scenario {
	t.Helper()
	client, _, upstreams := overPrivyFakes(t)
	users, wallets := privyadapter.Users{Client: client}, privyadapter.Wallets{Client: client}
	return scenario.New(t, append([]scenario.Option{
		scenario.WithModules(func(d module.Deps) module.Module {
			return identity.New(d, identity.WithPrivy(users, wallets))
		}),
		scenario.WithPrivy(upstreams, privyAppID),
	}, extra...)...)
}

func TestFlow01_OpenSession_OK(t *testing.T) {
	t.Parallel()
	flows.F01OpenSessionOK(identityScenario(t))
}

func TestFlow01_OpenSession_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F01OpenSessionUnauthorized(identityScenario(t))
}

func TestFlow01_OpenSession_LoginMethodNotAllowed(t *testing.T) {
	t.Parallel()
	flows.F01OpenSessionLoginMethodNotAllowed(identityScenario(t))
}

func TestFlow01_OpenSession_AccountDeleted(t *testing.T) {
	t.Parallel()
	flows.F01OpenSessionAccountDeleted(identityScenario(t))
}

func TestFlow01_OpenSession_PrivyUnavailable(t *testing.T) {
	t.Parallel()
	flows.F01OpenSessionPrivyUnavailable(identityScenario(t))
}
