package funding_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow05_CreditDeposit_OK(t *testing.T) {
	t.Parallel()
	s := flow05Scenario(t)
	flows.F05CreditDepositOK(s)
	s.Then(scenario.EventuallyEvent(events.TypeDepositCredited))
	var id uuid.UUID
	var signature string
	var createdAt time.Time
	if err := s.DB().QueryRow(t.Context(),
		`SELECT id, tx_signature, created_at FROM user_txns`).Scan(&id, &signature, &createdAt); err != nil {
		t.Fatal(err)
	}
	s.When(
		scenario.Get("/v1/me/txns"),
	).Then(
		scenario.ExpectStatus(200),
		scenario.ExpectJSON("items", []any{map[string]any{
			"id": id, "kind": "deposit", "status": "settled", "usdc_micros": "27500000", "cabal": nil,
			"tx_signature": signature, "created_at": createdAt.UTC().Format(time.RFC3339Nano),
		}}),
		scenario.ExpectJSON("next_cursor", nil),
	)
}

func TestFlow05_CreditDeposit_RPCUnavailable(t *testing.T) {
	t.Parallel()
	flows.F05CreditDepositRPCUnavailable(flow05Scenario(t))
}

func TestFlow05_CreditDeposit_NotADeposit(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	candidate := f.candidate()
	if got := newCandidateDispatch(t, f, candidateRPC{}).deliver(t, candidate); got != bus.OutcomeAck {
		t.Fatalf("delivery = %q, want ack", got)
	}
	if got := f.candidateStatus(t, candidate.Signature); got != "not_deposit" {
		t.Fatalf("status = %q, want not_deposit", got)
	}
}

func TestFlow05_CreditDeposit_MonacoSigned(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	candidate := f.candidate()
	d := newCandidateDispatch(t, f, candidateRPC{transfers: []solana.Transfer{{
		Mint: chain.Mint{Address: testkit.USDCMint, Decimals: 6}, Net: money.NewBaseUnits(1, 6),
	}}}, candidateOwner{owned: true})
	if got := d.deliver(t, candidate); got != bus.OutcomeAck {
		t.Fatalf("delivery = %q, want ack", got)
	}
	if got := f.candidateStatus(t, candidate.Signature); got != "ours" {
		t.Fatalf("status = %q, want ours", got)
	}
}

func TestFlow05_CreditDeposit_Unresolved(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	candidate := f.candidate()
	d := newCandidateDispatch(t, f, candidateRPC{err: errs.New(errs.CodeDecodeFailed, "test.rpc")})
	if got := d.deliver(t, candidate); got != bus.OutcomeTerm {
		t.Fatalf("delivery = %q, want term", got)
	}
	if got := f.candidateStatus(t, candidate.Signature); got != "pending" {
		t.Fatalf("status = %q, want pending", got)
	}
}

func flow05Scenario(t *testing.T) *scenario.Scenario {
	t.Helper()
	upstreams := fakes.New()
	srv := httptest.NewServer(upstreams)
	t.Cleanup(srv.Close)
	return scenario.New(t,
		scenario.WithPrivy(upstreams, "app"),
		scenario.WithModules(func(d module.Deps) module.Module {
			d.Config = config.Config{
				Solana:   config.Solana{RPCURL: srv.URL + "/rpc/", USDCMint: string(testkit.USDCMint)},
				Funding:  config.Funding{DepositPollInterval: app.DepositPollInterval, DepositRPCRate: 1000},
				Timeouts: config.Timeouts{RPC: time.Second},
			}
			d.HTTPClient = httpclient.New
			return funding.New(d)
		}, func(d module.Deps) module.Module {
			d.Config.Solana.USDCMint = string(testkit.USDCMint)
			return treasury.New(d)
		}),
	)
}
