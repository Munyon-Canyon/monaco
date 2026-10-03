package funding_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
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
	var r httpx.Routes
	m.Routes(&r)
	if r.FundingRoutes == nil {
		t.Fatal("Routes left FundingRoutes unset")
	}
	if got := m.Consumers(); len(got) != 0 {
		t.Fatalf("Consumers = %v, want none", got)
	}
	got := m.Pollers()
	if len(got) != 2 || got[0].Name() != "funding.deposits" || got[1].Name() != "funding.onramp-expiry" {
		t.Fatalf("Pollers = %v, want funding.deposits then funding.onramp-expiry", got)
	}
	if m.Balances() == nil {
		t.Fatal("Balances = nil")
	}
}
