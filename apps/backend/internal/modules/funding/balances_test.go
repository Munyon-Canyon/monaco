package funding_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
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
	if got := m.Consumers(); len(got) != 1 || got[0].Durable != "funding" || len(got[0].Handlers) != 1 ||
		got[0].Handlers[0].Name != "funding.resolve_deposit_candidate" {
		t.Fatalf("Consumers = %v, want funding.resolve_deposit_candidate", got)
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
	if owned, err := m.SignatureOwner().OwnsSignature(t.Context(), "signature"); err != nil || owned {
		t.Fatalf("SignatureOwner() = %t, %v, want false nil", owned, err)
	}
}

func TestBalance_ModuleBuildsWithoutRPCConfig(t *testing.T) {
	t.Parallel()
	if funding.New(module.Deps{}).Balances() == nil {
		t.Fatal("Balances = nil")
	}
}

func TestModule_ConsumersBuildWithoutRPCConfig(t *testing.T) {
	t.Parallel()
	if got := funding.New(module.Deps{}).Consumers(); len(got) != 1 {
		t.Fatalf("Consumers = %v, want one consumer", got)
	}
}

func TestModule_ConsumerCreatesTheSolanaReaderOnFirstFetch(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32015,"message":"unsupported"}}`))
	}))
	t.Cleanup(srv.Close)
	m := funding.New(module.Deps{Config: config.Config{
		Solana:   config.Solana{RPCURL: srv.URL, USDCMint: string(testkit.USDCMint)},
		Timeouts: config.Timeouts{RPC: time.Second},
	}, Clock: clock.Real{}})
	_, err := m.Consumers()[0].Handlers[0].Fetch(t.Context(), events.DepositCandidateSeen{
		WalletAddress: chain.AddressOf(make([]byte, 32)), TxSignature: "signature",
	})
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Fetch error = %v, want decode_failed", err)
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
