package funding_test

import (
	"net/http"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func requireBounceConsumer(t *testing.T, m *funding.Module) {
	t.Helper()
	got := m.Consumers()
	if len(got) != 1 || got[0].Durable != "funding_bounce" || len(got[0].Handlers) != 1 ||
		got[0].Handlers[0].Type() != events.TypeCabalExternalDepositDetected {
		t.Fatalf("Consumers = %v, want funding_bounce on cabal.external_deposit_detected", got)
	}
}

func TestModule(t *testing.T) {
	t.Parallel()
	m := funding.New(module.Deps{Config: config.Config{
		Solana:   config.Solana{RPCURL: "http://fakes/rpc/"},
		Funding:  config.Funding{DepositPollInterval: time.Minute, TreasuryReconcileInterval: 2 * time.Minute},
		Timeouts: config.Timeouts{RPC: time.Second},
	}})
	if got := m.Name(); got != "funding" {
		t.Fatalf("Name = %q, want funding", got)
	}
	for _, route := range [][2]string{{"GET", "/v1/me/balance"}, {"POST", "/v1/onramp/sessions"}} {
		if !testkit.Serves(m.Mount, route[0], route[1]) {
			t.Fatalf("Mount does not serve %s %s", route[0], route[1])
		}
	}
	requireBounceConsumer(t, m)
	pollers := m.Pollers()
	names := make([]string, 0, len(pollers))
	for _, p := range pollers {
		names = append(names, p.Name()+"@"+p.Interval().String())
	}
	want := []string{
		"funding.deposits@1m0s", "funding.onramp-expiry@1m0s", "funding.withdrawals@5s", "funding.bounce-sweeper@30s",
		"funding.treasury-reconcile@2m0s",
	}
	if !slices.Equal(names, want) {
		t.Fatalf("Pollers = %v, want %v", names, want)
	}
	if m.Balances() == nil {
		t.Fatal("Balances = nil")
	}
	if m.Pauses() == nil {
		t.Fatal("Pauses = nil")
	}
	if reflect.TypeOf(m.SignatureOwner()).Name() != "BounceSignatures" {
		t.Fatalf("SignatureOwner() = %T, want adapters.BounceSignatures", m.SignatureOwner())
	}
}

func TestBalance_ModuleBuildsWithoutRPCConfig(t *testing.T) {
	t.Parallel()
	if funding.New(module.Deps{}).Balances() == nil {
		t.Fatal("Balances = nil")
	}
}

func TestBalance_RPCUnavailable(t *testing.T) {
	t.Parallel()
	s := flow05Scenario(t)
	user := testkit.SeedUser(t, s.DB(), testkit.UserOpts{WithWallet: true})
	s.Given(
		scenario.AsSeededUser("member", user.ID),
		scenario.FakeUpstream(fakes.Step{
			Route: "/rpc/getTokenAccountsByOwner", Action: fakes.ActionFail, Status: http.StatusServiceUnavailable,
			Times: 100, Reset: true,
		}),
	).When(
		scenario.Get("/v1/me/balance"),
	).Then(
		scenario.ExpectStatus(http.StatusServiceUnavailable),
		scenario.ExpectProblem(errs.CodeRPCUnavailable),
	)
}

func TestBalance_RouteMatchesTheContractResponse(t *testing.T) {
	t.Parallel()
	s := flow05Scenario(t)
	user := testkit.SeedUser(t, s.DB(), testkit.UserOpts{WithWallet: true})
	s.Given(
		scenario.AsSeededUser("member", user.ID),
		scenario.FakeUpstream(fakes.Step{
			Route: "/rpc/getTokenAccountsByOwner", Action: fakes.ActionSucceed, Times: 100, Reset: true,
		}),
	).When(
		scenario.Get("/v1/me/balance"),
	).Then(
		scenario.ExpectStatus(http.StatusOK),
		scenario.ExpectJSON("available_micros", "25500000"),
		scenario.ExpectJSON("on_chain_micros", "25500000"),
		scenario.ExpectJSON("in_flight_micros", "0"),
		scenario.ExpectJSON("deposit_address", user.Address),
	)
}
