package funding_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func requireBounceConsumer(t *testing.T, m *funding.Module) {
	t.Helper()
	got := m.Consumers()
	if len(got) != 2 || got[0].Durable != "funding" || len(got[0].Handlers) != 1 ||
		got[0].Handlers[0].Name != "funding.resolve_deposit_candidate" ||
		got[0].Handlers[0].Type() != events.TypeDepositCandidateSeen {
		t.Fatalf("Consumers = %v, want funding resolving deposit.candidate_seen first", got)
	}
	if got[1].Durable != "funding_bounce" || len(got[1].Handlers) != 1 ||
		got[1].Handlers[0].Type() != events.TypeCabalExternalDepositDetected {
		t.Fatalf("Consumers = %v, want funding_bounce on cabal.external_deposit_detected", got)
	}
}

func TestModule(t *testing.T) {
	t.Parallel()
	m := funding.New(module.Deps{Config: config.Config{
		Solana: config.Solana{RPCURL: "http://fakes/rpc/"},
		Funding: config.Funding{
			DepositPollInterval: time.Minute, TreasuryReconcileInterval: 2 * time.Minute,
			BounceSweepInterval: 45 * time.Second,
		},
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
		"funding.deposit_watch@1m0s", "funding.onramp-expiry@1m0s", "funding.withdrawals@5s",
		"funding.bounce-sweeper@45s", "funding.treasury-reconcile@2m0s",
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

func finalizedChain(t *testing.T, readingSlot, finalizedAt uint64) (string, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var methods []string
	results := map[string]string{
		"getTokenAccountsByOwner": fmt.Sprintf(`{"context":{"slot":%d},"value":[{"account":{"data":{"parsed":`+
			`{"info":{"tokenAmount":{"amount":"60000000","decimals":6}}}}}}]}`, readingSlot),
		"getBlockHeight": `1`,
		"getSignatureStatuses": fmt.Sprintf(`{"context":{"slot":%d},"value":[{"slot":%d,"confirmations":null,`+
			`"err":null,"confirmationStatus":"finalized"}]}`, finalizedAt+30, finalizedAt),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&call)
		mu.Lock()
		methods = append(methods, call.Method)
		mu.Unlock()
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":%s}`, results[call.Method])
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/", func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(methods)
	}
}

func TestBalance_AvailableSubtractsAWithdrawalFinalizedBeforeThePollerTicksOnce(t *testing.T) {
	t.Parallel()
	const reading = 451_000_000
	for name, tc := range map[string]struct {
		status      string
		finalizedAt uint64
		available   uint64
		methods     []string
	}{
		"finalized at the reading's slot": {
			status: "submitted", finalizedAt: reading, available: 60_000_000,
			methods: []string{"getTokenAccountsByOwner", "getBlockHeight", "getSignatureStatuses"},
		},
		"finalized one slot after the reading": {
			status: "submitted", finalizedAt: reading + 1, available: 20_000_000,
			methods: []string{"getTokenAccountsByOwner", "getBlockHeight", "getSignatureStatuses"},
		},
		"created and not yet signed": {
			status: "created", finalizedAt: reading, available: 20_000_000,
			methods: []string{"getTokenAccountsByOwner"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pool := testkit.DB(t)
			user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
			if _, err := pool.Exec(t.Context(), seedWithdrawal,
				ids.Real{}.NewV7(), user.ID.UUID(), 40_000_000, tc.status, clock.Real{}.Now()); err != nil {
				t.Fatal(err)
			}
			url, methods := finalizedChain(t, reading, tc.finalizedAt)
			balances := funding.New(module.Deps{
				Config: config.Config{
					Solana:   config.Solana{RPCURL: url, USDCMint: string(testkit.USDCMint)},
					Timeouts: config.Timeouts{RPC: time.Second},
				},
				Pool: pool, Clock: testkit.NewClock(clock.Real{}.Now()),
			}).Balances()
			got, err := balances.Available(t.Context(), user.ID)
			if err != nil || got.OnChainMicros.Uint64() != 60_000_000 || got.AvailableMicros.Uint64() != tc.available {
				t.Fatalf("Available = %+v, %v, want 60 USDC on chain and %d micros available", got, err, tc.available)
			}
			if !slices.Equal(methods(), tc.methods) {
				t.Fatalf("RPC calls = %v, want %v", methods(), tc.methods)
			}
		})
	}
}
