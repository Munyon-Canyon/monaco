package identity_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestModule_registersItsRoutesAndTheNudgePoller(t *testing.T) {
	t.Parallel()
	cfg := privyConfig()
	cfg.Identity.NudgesInterval = 5 * time.Hour
	m := identity.New(module.Deps{Config: cfg, Clock: clock.Real{}},
		identity.WithHoldings(fakes.NewTreasury(), fakes.NewBalances()))
	if m.Name() != "identity" || !testkit.Serves(m.Mount, "POST", "/v1/auth/session") {
		t.Fatalf("module %s does not serve POST /v1/auth/session", m.Name())
	}
	consumers := m.Consumers()
	if len(consumers) != 1 || consumers[0].Durable != "identity_first_deposit" ||
		len(consumers[0].Handlers) != 1 || consumers[0].Handlers[0].Name != "identity.first_deposit" {
		t.Fatalf("consumers = %v, want identity_first_deposit with handler identity.first_deposit", consumers)
	}
	pollers := m.Pollers()
	if len(pollers) != 1 || pollers[0].Name() != "identity.nudges" || pollers[0].Interval() != 5*time.Hour {
		t.Fatalf("pollers = %v, want identity.nudges every IDENTITY_NUDGES_INTERVAL", pollers)
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
	testkit.Serves(m.Mount, "GET", "/")
	t.Fatal("Mount did not panic")
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
	testkit.Serves(identity.New(module.Deps{Config: cfg, Clock: clock.Real{}}).Mount, "GET", "/")
	t.Fatal("Mount did not panic")
}

func TestModule_wireTakesHoldingsFromFundingAndTreasuryInTheBuiltSet(t *testing.T) {
	t.Parallel()
	d := module.Deps{}
	id := identity.New(d)
	fundingModule := funding.New(d)
	module.NewSet(id, fundingModule, treasury.New(d))
	stakes, balances := id.Holdings()
	if reflect.TypeOf(stakes) != reflect.TypeOf(treasury.New(d).Queries()) || balances != fundingModule.Balances() {
		t.Fatalf("holdings = %T, %T, want treasury's Queries and funding's Balances", stakes, balances)
	}
}

func TestModule_holdingsFailClosedWhenFundingAndTreasuryAreNotInTheSet(t *testing.T) {
	t.Parallel()
	id := identity.New(module.Deps{})
	module.NewSet(id)
	stakes, balances := id.Holdings()
	user := ids.UserID{}
	if _, err := stakes.(app.Stakes).StakesOf(t.Context(), user); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("StakesOf err = %v, want upstream_unavailable", err)
	}
	_, err := balances.(app.Balances).Available(t.Context(), user)
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Available err = %v, want upstream_unavailable", err)
	}
}

func TestModule_withHoldingsWinsOverWire(t *testing.T) {
	t.Parallel()
	d := module.Deps{}
	treasuryFake, balancesFake := fakes.NewTreasury(), fakes.NewBalances()
	id := identity.New(d, identity.WithHoldings(treasuryFake, balancesFake))
	module.NewSet(id, funding.New(d), treasury.New(d))
	stakes, balances := id.Holdings()
	if stakes != any(treasuryFake) || balances != any(balancesFake) {
		t.Fatalf("holdings = %T, %T, want the injected fakes", stakes, balances)
	}
}
