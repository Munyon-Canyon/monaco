package identity_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestModule_registersItsRoutesAndNoConsumersOrPollersYet(t *testing.T) {
	t.Parallel()
	m := identity.New(module.Deps{Config: privyConfig(), Clock: clock.Real{}},
		identity.WithHoldings(fakes.NewTreasury(), fakes.NewBalances()))
	var routes httpx.Routes
	m.Routes(&routes)
	if m.Name() != "identity" || routes.IdentityRoutes == nil || len(m.Consumers()) != 0 || m.Pollers() != nil {
		t.Fatalf("module = %s, routes %+v, consumers %v, pollers %v", m.Name(), routes, m.Consumers(), m.Pollers())
	}
}

func TestModule_routesPanicWhenTheWalletMeterCannotBeCreated(t *testing.T) {
	t.Parallel()
	defer func() {
		err, ok := recover().(error)
		if !ok || errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("Routes panicked with %v, want an internal error", err)
		}
	}()
	m := identity.New(module.Deps{Config: privyConfig(), Clock: clock.Real{}},
		identity.WithMeters(testkit.FailingGauges{Prefix: "identity_"}))
	var routes httpx.Routes
	m.Routes(&routes)
	t.Fatal("Routes did not panic")
}

func TestModule_routesPanicWithoutAPrivyVerificationKey(t *testing.T) {
	t.Parallel()
	defer func() {
		err, ok := recover().(error)
		if !ok || errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("Routes panicked with %v, want invalid_input from privy.New", err)
		}
	}()
	cfg := privyConfig()
	cfg.Privy.VerificationKey = ""
	var routes httpx.Routes
	identity.New(module.Deps{Config: cfg, Clock: clock.Real{}}).Routes(&routes)
	t.Fatal("Routes did not panic")
}
