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
	m.Routes(&httpx.Routes{})
	if got := m.Consumers(); len(got) != 0 {
		t.Fatalf("Consumers = %v, want none", got)
	}
	if got := m.Pollers(); len(got) != 1 || got[0].Name() != "funding.deposits" {
		t.Fatalf("Pollers = %v, want funding.deposits", got)
	}
	if m.Balances() == nil {
		t.Fatal("Balances = nil")
	}
}
