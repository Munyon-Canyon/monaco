package funding_test

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestModule(t *testing.T) {
	t.Parallel()
	m := funding.New(module.Deps{Config: config.Config{
		Solana:   config.Solana{RPCURL: "http://fakes/rpc/"},
		Funding:  config.Funding{DepositPollInterval: time.Minute},
		Timeouts: config.Timeouts{RPC: time.Second},
	}})
	if got := m.Name(); got != "funding" {
		t.Fatalf("Name = %q, want funding", got)
	}
	if !testkit.Serves(m.Mount, "GET", "/v1/me/balance") {
		t.Fatal("Mount does not serve GET /v1/me/balance")
	}
	if got := m.Consumers(); len(got) != 0 {
		t.Fatalf("Consumers = %v, want none", got)
	}
	if got := m.Pollers(); len(got) != 1 || got[0].Name() != "funding.deposits" {
		t.Fatalf("Pollers = %v, want funding.deposits", got)
	}
	if m.Balances() == nil {
		t.Fatal("Balances = nil")
	}
	if m.Pauses() == nil {
		t.Fatal("Pauses = nil")
	}
	if reflect.TypeOf(m.SignatureOwner()).Name() != "UnwiredSignatureOwner" {
		t.Fatalf("SignatureOwner() = %T, want adapters.UnwiredSignatureOwner", m.SignatureOwner())
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
